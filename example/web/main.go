// Package main implements a pure Go web dashboard for WhatsRook using github.com/Thruqe/htmlbuilder.
// It bridges browser WebSockets with the WhatsRook daemon's protobuf WebSocket protocol.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/Thruqe/htmlbuilder"
	"github.com/coder/websocket"
	"google.golang.org/protobuf/proto"

	"whatsrook/proto/wsproto"
)

type DaemonBridge struct {
	mu             sync.Mutex
	daemonConn     *websocket.Conn
	browserClients map[*websocket.Conn]bool
	lastStatus     map[string]any
	daemonURL      string
}

func NewDaemonBridge(daemonURL string) *DaemonBridge {
	return &DaemonBridge{
		browserClients: make(map[*websocket.Conn]bool),
		daemonURL:      daemonURL,
	}
}

func (b *DaemonBridge) Start(ctx context.Context) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				b.connectToDaemon(ctx)
				time.Sleep(3 * time.Second)
			}
		}
	}()
}

func (b *DaemonBridge) connectToDaemon(ctx context.Context) {
	log.Printf("[daemon] connecting to %s...", b.daemonURL)

	opts := &websocket.DialOptions{
		Subprotocols: []string{"protobuf"},
	}

	conn, _, err := websocket.Dial(ctx, b.daemonURL, opts)
	if err != nil {
		log.Printf("[daemon] connection failed: %v", err)
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "disconnecting")

	b.mu.Lock()
	b.daemonConn = conn
	b.mu.Unlock()

	log.Printf("[daemon] connected successfully")

	// Send initial get_status request
	b.sendControl(&wsproto.ControlFrame{
		Type: wsproto.ControlType_CONTROL_TYPE_GET_STATUS,
		Id:   reqID("status"),
		Payload: &wsproto.ControlFrame_GetStatus{
			GetStatus: &wsproto.GetStatusPayload{},
		},
	})

	for {
		typ, data, err := conn.Read(ctx)
		if err != nil {
			log.Printf("[daemon] read error / disconnected: %v", err)
			b.mu.Lock()
			b.daemonConn = nil
			b.mu.Unlock()
			return
		}

		if typ != websocket.MessageBinary {
			continue
		}

		var frame wsproto.EventFrame
		if err := proto.Unmarshal(data, &frame); err != nil {
			log.Printf("[daemon] failed to decode EventFrame: %v", err)
			continue
		}

		eventObj := b.frameToBrowserEvent(&frame)
		if eventObj != nil {
			if eventObj["type"] == "status" {
				b.mu.Lock()
				b.lastStatus = eventObj["data"].(map[string]any)
				b.mu.Unlock()
			}
			b.broadcastToBrowsers(eventObj)
		}
	}
}

func (b *DaemonBridge) sendControl(frame *wsproto.ControlFrame) {
	b.mu.Lock()
	conn := b.daemonConn
	b.mu.Unlock()

	if conn == nil {
		log.Printf("[daemon] cannot send control, daemon not connected")
		return
	}

	data, err := proto.Marshal(frame)
	if err != nil {
		log.Printf("[daemon] failed to marshal ControlFrame: %v", err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := conn.Write(ctx, websocket.MessageBinary, data); err != nil {
		log.Printf("[daemon] failed to write ControlFrame: %v", err)
	}
}

func (b *DaemonBridge) frameToBrowserEvent(frame *wsproto.EventFrame) map[string]any {
	switch p := frame.Payload.(type) {
	case *wsproto.EventFrame_Status:
		s := p.Status
		var jid *string
		if s.GetJid() != "" {
			val := s.GetJid()
			jid = &val
		}
		var pushName *string
		if s.GetPushName() != "" {
			val := s.GetPushName()
			pushName = &val
		}
		return map[string]any{
			"type": "status",
			"data": map[string]any{
				"connected": s.Connected,
				"loggedIn":  s.LoggedIn,
				"jid":       jid,
				"pushName":  pushName,
			},
		}

	case *wsproto.EventFrame_PairQr:
		return map[string]any{
			"type": "pair_qr",
			"data": map[string]any{
				"code": p.PairQr.GetCode(),
			},
		}

	case *wsproto.EventFrame_PairCode:
		return map[string]any{
			"type": "pair_code",
			"data": map[string]any{
				"code": p.PairCode.GetCode(),
			},
		}

	case *wsproto.EventFrame_PairError:
		return map[string]any{
			"type": "pair_error",
			"data": map[string]any{
				"reason": p.PairError.GetReason(),
			},
		}

	case *wsproto.EventFrame_Ack:
		return map[string]any{
			"type": "ack",
			"data": map[string]any{
				"ok":    p.Ack.GetOk(),
				"error": p.Ack.GetError(),
			},
		}

	case *wsproto.EventFrame_Message:
		m := p.Message
		return map[string]any{
			"type": "message",
			"data": map[string]any{
				"from":      m.GetFrom(),
				"chat":      m.GetChat(),
				"sender":    m.GetSender(),
				"text":      m.GetText(),
				"messageId": m.GetMessageId(),
				"pushName":  m.GetPushName(),
				"timestamp": m.GetTimestampUnix(),
				"isGroup":   m.GetIsGroup(),
				"isFromMe":  m.GetIsFromMe(),
			},
		}

	case *wsproto.EventFrame_IncomingCall:
		c := p.IncomingCall
		return map[string]any{
			"type": "incoming_call",
			"data": map[string]any{
				"callId": c.GetCallId(),
				"from":   c.GetFrom(),
			},
		}

	default:
		return nil
	}
}

func (b *DaemonBridge) registerBrowser(ws *websocket.Conn) {
	b.mu.Lock()
	b.browserClients[ws] = true
	lastSt := b.lastStatus
	b.mu.Unlock()

	log.Printf("[browser] client connected")

	// If we have a cached status, send it immediately
	if lastSt != nil {
		msg, _ := json.Marshal(map[string]any{
			"type": "status",
			"data": lastSt,
		})
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		_ = ws.Write(ctx, websocket.MessageText, msg)
		cancel()
	}
}

func (b *DaemonBridge) unregisterBrowser(ws *websocket.Conn) {
	b.mu.Lock()
	delete(b.browserClients, ws)
	b.mu.Unlock()
	log.Printf("[browser] client disconnected")
}

func (b *DaemonBridge) broadcastToBrowsers(eventObj map[string]any) {
	data, err := json.Marshal(eventObj)
	if err != nil {
		return
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	for ws := range b.browserClients {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		_ = ws.Write(ctx, websocket.MessageText, data)
		cancel()
	}
}

func (b *DaemonBridge) handleBrowserMessage(message []byte) {
	var msg struct {
		Action  string         `json:"action"`
		Payload map[string]any `json:"payload"`
	}
	if err := json.Unmarshal(message, &msg); err != nil {
		return
	}

	switch msg.Action {
	case "request_pair_code":
		phone := ""
		if val, ok := msg.Payload["phoneNumber"].(string); ok {
			phone = val
		}
		b.sendControl(&wsproto.ControlFrame{
			Type: wsproto.ControlType_CONTROL_TYPE_REQUEST_PAIR_CODE,
			Id:   reqID("pair"),
			Payload: &wsproto.ControlFrame_RequestPairCode{
				RequestPairCode: &wsproto.RequestPairCodePayload{
					PhoneNumber: phone,
				},
			},
		})

	case "request_pair_qr":
		b.sendControl(&wsproto.ControlFrame{
			Type: wsproto.ControlType_CONTROL_TYPE_REQUEST_PAIR_QR,
			Id:   reqID("qr"),
			Payload: &wsproto.ControlFrame_RequestPairQr{
				RequestPairQr: &wsproto.RequestPairQRPayload{},
			},
		})

	case "get_status":
		b.sendControl(&wsproto.ControlFrame{
			Type: wsproto.ControlType_CONTROL_TYPE_GET_STATUS,
			Id:   reqID("status"),
			Payload: &wsproto.ControlFrame_GetStatus{
				GetStatus: &wsproto.GetStatusPayload{},
			},
		})

	case "logout":
		b.sendControl(&wsproto.ControlFrame{
			Type: wsproto.ControlType_CONTROL_TYPE_LOGOUT,
			Id:   reqID("logout"),
			Payload: &wsproto.ControlFrame_Logout{
				Logout: &wsproto.LogoutPayload{},
			},
		})

	case "disconnect":
		b.sendControl(&wsproto.ControlFrame{
			Type: wsproto.ControlType_CONTROL_TYPE_DISCONNECT,
			Id:   reqID("disconnect"),
			Payload: &wsproto.ControlFrame_Disconnect{
				Disconnect: &wsproto.DisconnectPayload{},
			},
		})
	}
}

func reqID(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
}

func main() {
	daemonURL := os.Getenv("WHATSROOK_WS_URL")
	if daemonURL == "" {
		daemonURL = "ws://localhost:8080/ws"
	}

	preferredPort := 3000
	if portStr := os.Getenv("PORT"); portStr != "" {
		if p, err := strconv.Atoi(portStr); err == nil {
			preferredPort = p
		}
	}

	bridge := NewDaemonBridge(daemonURL)
	bridge.Start(context.Background())

	mux := http.NewServeMux()

	// Serve the interactive Web Dashboard generated by htmlbuilder
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintln(w, renderDashboardPage())
	})

	// Serve Browser WebSocket Bridge Endpoint
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			InsecureSkipVerify: true,
		})
		if err != nil {
			log.Printf("[browser] websocket accept error: %v", err)
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "")

		bridge.registerBrowser(conn)
		defer bridge.unregisterBrowser(conn)

		ctx := r.Context()
		for {
			typ, data, err := conn.Read(ctx)
			if err != nil {
				break
			}
			if typ == websocket.MessageText {
				bridge.handleBrowserMessage(data)
			}
		}
	})

	// Find an open port starting from preferredPort
	port := preferredPort
	var listener net.Listener
	for {
		addr := fmt.Sprintf(":%d", port)
		l, err := net.Listen("tcp", addr)
		if err == nil {
			listener = l
			break
		}
		log.Printf("[server] Port %d in use, trying %d...", port, port+1)
		port++
	}

	log.Printf("Dashboard server running at http://localhost:%d", port)
	if err := http.Serve(listener, mux); err != nil {
		log.Fatalf("Server exit: %v", err)
	}
}

func renderDashboardPage() string {
	doc := htmlbuilder.New().
		Title("WhatsRook — Dashboard").
		MetaDefault().
		Link(map[string]string{
			"rel":  "stylesheet",
			"href": "https://fonts.googleapis.com/css2?family=Plus+Jakarta+Sans:wght@400;500;600;700;800&display=swap",
		}).
		Link(map[string]string{
			"rel":  "stylesheet",
			"href": "https://cdn-uicons.flaticon.com/2.6.0/uicons-solid-rounded/css/uicons-solid-rounded.css",
		}).
		Link(map[string]string{
			"rel":  "stylesheet",
			"href": "https://cdn-uicons.flaticon.com/2.6.0/uicons-regular-rounded/css/uicons-regular-rounded.css",
		}).
		StyleBlock(`
			:root {
				--brand: #25d366;
				--brand-dark: #128c7e;
				--brand-soft: #e8f8f0;
				--bg: #f8fafc;
				--card-bg: #ffffff;
				--text: #0f172a;
				--text-dim: #64748b;
				--border: #e2e8f0;
				--danger: #ef4444;
			}
			* {
				box-sizing: border-box;
				margin: 0;
				padding: 0;
			}
			body {
				font-family: 'Plus Jakarta Sans', -apple-system, BlinkMacSystemFont, sans-serif;
				background: var(--bg);
				color: var(--text);
				min-height: 100vh;
				display: flex;
				align-items: center;
				justify-content: center;
				padding: 20px;
			}
			.screen {
				display: none;
				width: 100%;
				max-width: 440px;
			}
			.screen.active {
				display: block;
			}
			.card {
				background: var(--card-bg);
				border: 1px solid var(--border);
				border-radius: 24px;
				padding: 36px 32px;
				box-shadow: 0 20px 40px rgba(15, 23, 42, 0.05);
			}
			.brand {
				display: flex;
				align-items: center;
				gap: 10px;
				margin-bottom: 24px;
			}
			.brand-icon {
				width: 40px;
				height: 40px;
				border-radius: 12px;
				background: var(--brand-soft);
				color: var(--brand-dark);
				display: flex;
				align-items: center;
				justify-content: center;
				font-size: 20px;
			}
			.brand-name {
				font-weight: 800;
				font-size: 18px;
				color: var(--text);
			}
			h1 {
				font-size: 24px;
				font-weight: 800;
				letter-spacing: -0.02em;
				margin-bottom: 8px;
			}
			p.sub {
				color: var(--text-dim);
				font-size: 14px;
				line-height: 1.5;
				margin-bottom: 28px;
			}
			.field {
				margin-bottom: 20px;
			}
			label {
				display: block;
				font-size: 13px;
				font-weight: 700;
				color: var(--text-dim);
				margin-bottom: 8px;
			}
			.phone-input {
				display: flex;
				align-items: center;
				border: 1.5px solid var(--border);
				border-radius: 12px;
				padding: 10px 14px;
				background: #fff;
				transition: border-color 0.15s;
			}
			.phone-input:focus-within {
				border-color: var(--brand-dark);
			}
			.phone-input .prefix {
				color: var(--text-dim);
				font-size: 16px;
				margin-right: 10px;
			}
			.phone-input input {
				border: none;
				outline: none;
				font-size: 16px;
				font-weight: 600;
				width: 100%;
				font-family: inherit;
			}
			button.primary {
				width: 100%;
				background: var(--brand-dark);
				color: #fff;
				border: none;
				border-radius: 12px;
				padding: 14px;
				font-size: 15px;
				font-weight: 700;
				cursor: pointer;
				transition: opacity 0.15s, transform 0.1s;
				display: flex;
				align-items: center;
				justify-content: center;
				gap: 8px;
			}
			button.primary:hover {
				opacity: 0.92;
			}
			button.primary:active {
				transform: scale(0.99);
			}
			.error-msg {
				display: none;
				align-items: center;
				gap: 8px;
				color: var(--danger);
				font-size: 13px;
				font-weight: 600;
				margin-top: 14px;
			}
			.error-msg.active {
				display: flex;
			}
			.hint {
				font-size: 12.5px;
				color: var(--text-dim);
				margin-top: 20px;
				text-align: center;
			}
			.back-link {
				display: inline-flex;
				align-items: center;
				gap: 6px;
				font-size: 13.5px;
				font-weight: 700;
				color: var(--text-dim);
				cursor: pointer;
				margin-bottom: 20px;
				transition: color 0.15s;
			}
			.back-link:hover {
				color: var(--text);
			}
			.choice-grid {
				display: grid;
				grid-template-columns: 1fr 1fr;
				gap: 14px;
			}
			.choice-card {
				border: 1.5px solid var(--border);
				border-radius: 16px;
				padding: 20px 16px;
				text-align: center;
				cursor: pointer;
				transition: border-color 0.15s, background 0.15s;
			}
			.choice-card:hover {
				border-color: var(--brand-dark);
				background: var(--brand-soft);
			}
			.choice-card i {
				font-size: 28px;
				color: var(--brand-dark);
				margin-bottom: 10px;
				display: inline-block;
			}
			.choice-card .title {
				font-weight: 700;
				font-size: 14px;
				margin-bottom: 4px;
			}
			.choice-card .desc {
				font-size: 12px;
				color: var(--text-dim);
			}
			.pair-visual {
				display: flex;
				justify-content: center;
				align-items: center;
				min-height: 220px;
				margin-bottom: 20px;
			}
			.pair-code-display {
				display: flex;
				gap: 12px;
				justify-content: center;
				flex-wrap: wrap;
			}
			.pair-code-display span {
				font-size: 26px;
				font-weight: 800;
				letter-spacing: 0.05em;
				background: var(--brand-soft);
				color: var(--brand-dark);
				padding: 10px 14px;
				border-radius: 10px;
				font-variant-numeric: tabular-nums;
			}
			.dot-status {
				display: inline-flex;
				align-items: center;
				gap: 6px;
				font-size: 13px;
				color: var(--text-dim);
				font-weight: 600;
			}
			.dot {
				width: 8px;
				height: 8px;
				border-radius: 50%;
				background: #d1d5db;
			}
			.dot.online {
				background: var(--brand);
			}
			.dash-wrap {
				max-width: 520px;
			}
			.dash-header {
				display: flex;
				align-items: center;
				justify-content: space-between;
				margin-bottom: 24px;
			}
			.dash-profile {
				display: flex;
				align-items: center;
				gap: 12px;
			}
			.avatar {
				width: 44px;
				height: 44px;
				border-radius: 50%;
				background: var(--brand-soft);
				color: var(--brand-dark);
				display: flex;
				align-items: center;
				justify-content: center;
				font-weight: 700;
				font-size: 16px;
			}
			.dash-profile .name {
				font-weight: 700;
				font-size: 15px;
			}
			.dash-profile .status {
				font-size: 12.5px;
				color: var(--text-dim);
			}
			.icon-btn {
				width: 38px;
				height: 38px;
				border-radius: 10px;
				border: 1.5px solid var(--border);
				background: #fff;
				display: flex;
				align-items: center;
				justify-content: center;
				cursor: pointer;
				color: var(--text-dim);
				transition: border-color 0.15s, color 0.15s;
			}
			.icon-btn:hover {
				border-color: var(--danger);
				color: var(--danger);
			}
			.stat-row {
				display: grid;
				grid-template-columns: 1fr 1fr;
				gap: 12px;
				margin-bottom: 20px;
			}
			.stat-box {
				border: 1.5px solid var(--border);
				border-radius: 12px;
				padding: 14px 16px;
			}
			.stat-box .label {
				font-size: 12px;
				color: var(--text-dim);
				font-weight: 600;
				margin-bottom: 4px;
			}
			.stat-box .value {
				font-size: 15.5px;
				font-weight: 700;
			}
			.feed-title {
				font-size: 13.5px;
				font-weight: 700;
				color: var(--text-dim);
				margin: 6px 0 10px;
				text-transform: uppercase;
				letter-spacing: 0.04em;
			}
			.feed {
				display: flex;
				flex-direction: column;
				gap: 10px;
				max-height: 320px;
				overflow-y: auto;
			}
			.feed-item {
				border: 1.5px solid var(--border);
				border-radius: 12px;
				padding: 12px 14px;
			}
			.feed-item .top {
				display: flex;
				justify-content: space-between;
				font-size: 12.5px;
				color: var(--text-dim);
				margin-bottom: 4px;
			}
			.feed-item .body {
				font-size: 14px;
			}
			.feed-empty {
				text-align: center;
				color: var(--text-dim);
				font-size: 13.5px;
				padding: 24px 0;
			}
		`)

	// Screen 1: Phone number entry
	screenPhone := htmlbuilder.El("div").
		Class("screen", "active").
		Attr("id", "screen-phone").
		Child(
			htmlbuilder.El("div").Class("card").Child(
				htmlbuilder.El("div").Class("brand").Child(
					htmlbuilder.El("div").Class("brand-icon").Child(htmlbuilder.El("i").Class("fi", "fi-sr-comment-check")),
					htmlbuilder.El("div").Class("brand-name").Child(htmlbuilder.Span("WhatsRook")),
				),
				htmlbuilder.H1("Connect your number"),
				htmlbuilder.P("Enter your WhatsApp phone number to get started.").Class("sub"),
				htmlbuilder.El("div").Class("field").Child(
					htmlbuilder.El("label").Attr("for", "phone-number").Child(htmlbuilder.Span("Phone number")),
					htmlbuilder.El("div").Class("phone-input").Child(
						htmlbuilder.Span("").Class("prefix").Child(htmlbuilder.El("i").Class("fi", "fi-sr-phone-flip")),
						htmlbuilder.El("input").
							Attr("type", "tel").
							Attr("id", "phone-number").
							Attr("placeholder", "e.g. 2348012345678").
							Attr("inputmode", "numeric").
							Attr("autocomplete", "off"),
					),
				),
				htmlbuilder.El("button").Class("primary").Attr("id", "btn-continue").Child(
					htmlbuilder.Span("Continue").Attr("id", "btn-continue-text"),
				),
				htmlbuilder.El("div").Class("error-msg").Attr("id", "phone-error").Child(
					htmlbuilder.El("i").Class("fi", "fi-sr-triangle-warning"),
					htmlbuilder.Span("Something went wrong. Please try again.").Attr("id", "phone-error-text"),
				),
				htmlbuilder.P("Use your number in international format, digits only.").Class("hint"),
			),
		)

	// Screen 2: Choose QR or Pair Code
	screenChoice := htmlbuilder.El("div").
		Class("screen").
		Attr("id", "screen-choice").
		Child(
			htmlbuilder.El("div").Class("card").Child(
				htmlbuilder.El("div").Class("back-link").Attr("id", "choice-back").Child(
					htmlbuilder.El("i").Class("fi", "fi-sr-arrow-small-left"),
					htmlbuilder.Span(" Back"),
				),
				htmlbuilder.H1("Link your device"),
				htmlbuilder.P("Choose how you'd like to connect WhatsApp.").Class("sub"),
				htmlbuilder.El("div").Class("choice-grid").Child(
					htmlbuilder.El("div").Class("choice-card").Attr("id", "choice-qr").Child(
						htmlbuilder.El("i").Class("fi", "fi-sr-qrcode"),
						htmlbuilder.El("div").Class("title").Child(htmlbuilder.Span("Scan QR code")),
						htmlbuilder.El("div").Class("desc").Child(htmlbuilder.Span("Use WhatsApp on your phone to scan")),
					),
					htmlbuilder.El("div").Class("choice-card").Attr("id", "choice-pair").Child(
						htmlbuilder.El("i").Class("fi", "fi-sr-keyboard"),
						htmlbuilder.El("div").Class("title").Child(htmlbuilder.Span("Enter a code")),
						htmlbuilder.El("div").Class("desc").Child(htmlbuilder.Span("Get a code to type on your phone")),
					),
				),
			),
		)

	// Screen 3: QR pairing
	screenQR := htmlbuilder.El("div").
		Class("screen").
		Attr("id", "screen-qr").
		Child(
			htmlbuilder.El("div").Class("card").Child(
				htmlbuilder.El("div").Class("back-link").Attr("id", "qr-back").Child(
					htmlbuilder.El("i").Class("fi", "fi-sr-arrow-small-left"),
					htmlbuilder.Span(" Back"),
				),
				htmlbuilder.H1("Scan with WhatsApp"),
				htmlbuilder.P("Open WhatsApp → Linked Devices → Link a Device, then scan this code.").Class("sub"),
				htmlbuilder.El("div").Class("pair-visual").Child(
					htmlbuilder.El("div").Attr("id", "qr-canvas-wrap").Child(
						htmlbuilder.El("i").Class("fi", "fi-sr-spinner").SetStyle("font-size", "22px").SetStyle("color", "#c7cad6"),
					),
				),
				htmlbuilder.El("div").SetStyle("display", "flex").SetStyle("justify-content", "center").Child(
					htmlbuilder.El("span").Class("dot-status").Child(
						htmlbuilder.El("span").Class("dot").Attr("id", "qr-dot"),
						htmlbuilder.Span("Waiting for scan…").Attr("id", "qr-status-text"),
					),
				),
			),
		)

	// Screen 4: Pair code
	screenCode := htmlbuilder.El("div").
		Class("screen").
		Attr("id", "screen-code").
		Child(
			htmlbuilder.El("div").Class("card").Child(
				htmlbuilder.El("div").Class("back-link").Attr("id", "code-back").Child(
					htmlbuilder.El("i").Class("fi", "fi-sr-arrow-small-left"),
					htmlbuilder.Span(" Back"),
				),
				htmlbuilder.H1("Enter this code"),
				htmlbuilder.P("Open WhatsApp → Linked Devices → Link with phone number, then enter this code.").Class("sub"),
				htmlbuilder.El("div").Class("pair-visual").Child(
					htmlbuilder.El("div").Class("pair-code-display").Attr("id", "pair-code-display").Child(
						htmlbuilder.Span("••••"),
						htmlbuilder.Span("••••"),
					),
				),
				htmlbuilder.El("div").SetStyle("display", "flex").SetStyle("justify-content", "center").Child(
					htmlbuilder.El("span").Class("dot-status").Child(
						htmlbuilder.El("span").Class("dot").Attr("id", "code-dot"),
						htmlbuilder.Span("Waiting for confirmation…").Attr("id", "code-status-text"),
					),
				),
			),
		)

	// Screen 5: Dashboard
	screenDashboard := htmlbuilder.El("div").
		Class("screen").
		Attr("id", "screen-dashboard").
		Child(
			htmlbuilder.El("div").Class("card", "dash-wrap").Child(
				htmlbuilder.El("div").Class("dash-header").Child(
					htmlbuilder.El("div").Class("dash-profile").Child(
						htmlbuilder.El("div").Class("avatar").Child(htmlbuilder.El("i").Class("fi", "fi-sr-user")),
						htmlbuilder.El("div").Child(
							htmlbuilder.El("div").Class("name").Attr("id", "dash-name").Child(htmlbuilder.Span("Connected")),
							htmlbuilder.El("div").Class("status").Child(
								htmlbuilder.El("span").Class("dot", "online").SetStyle("display", "inline-block"),
								htmlbuilder.Span("Online").Attr("id", "dash-status"),
							),
						),
					),
					htmlbuilder.El("div").Class("icon-btn").Attr("id", "btn-logout").Attr("title", "Log out").Child(
						htmlbuilder.El("i").Class("fi", "fi-sr-sign-out-alt"),
					),
				),
				htmlbuilder.El("div").Class("stat-row").Child(
					htmlbuilder.El("div").Class("stat-box").Child(
						htmlbuilder.El("div").Class("label").Child(htmlbuilder.Span("Number")),
						htmlbuilder.El("div").Class("value").Attr("id", "dash-number").Child(htmlbuilder.Span("—")),
					),
					htmlbuilder.El("div").Class("stat-box").Child(
						htmlbuilder.El("div").Class("label").Child(htmlbuilder.Span("Status")),
						htmlbuilder.El("div").Class("value").Attr("id", "dash-conn-status").Child(htmlbuilder.Span("Connected")),
					),
				),
				htmlbuilder.El("div").Class("feed-title").Child(htmlbuilder.Span("Recent activity")),
				htmlbuilder.El("div").Class("feed").Attr("id", "dash-feed").Child(
					htmlbuilder.El("div").Class("feed-empty").Child(htmlbuilder.Span("No activity yet. Messages will show up here.")),
				),
			),
		)

	// Interactive JavaScript for WebSocket bridge & UI screen switching
	scriptBlock := `
		(function() {
			let ws = null;
			let userPhone = '';
			let currentStatus = null;

			const screens = {
				phone: document.getElementById('screen-phone'),
				choice: document.getElementById('screen-choice'),
				qr: document.getElementById('screen-qr'),
				code: document.getElementById('screen-code'),
				dashboard: document.getElementById('screen-dashboard'),
			};

			function showScreen(name) {
				Object.keys(screens).forEach(k => {
					if (screens[k]) screens[k].classList.toggle('active', k === name);
				});
			}

			function initWS() {
				const protocol = location.protocol === 'https:' ? 'wss:' : 'ws:';
				ws = new WebSocket(protocol + '//' + location.host + '/ws');

				ws.onopen = () => {
					console.log('[ws] connected');
					sendAction('get_status');
				};

				ws.onmessage = (evt) => {
					try {
						const event = JSON.parse(evt.data);
						handleEvent(event);
					} catch(err) {
						console.error('[ws] error parsing json:', err);
					}
				};

				ws.onclose = () => {
					console.log('[ws] closed, retrying in 3s...');
					setTimeout(initWS, 3000);
				};
			}

			function sendAction(action, payload = {}) {
				if (ws && ws.readyState === WebSocket.OPEN) {
					ws.send(JSON.stringify({ action, payload }));
				}
			}

			function handleEvent(event) {
				switch (event.type) {
					case 'status':
						currentStatus = event.data;
						if (currentStatus.loggedIn) {
							document.getElementById('dash-name').textContent = currentStatus.pushName || 'WhatsApp User';
							document.getElementById('dash-number').textContent = currentStatus.jid || 'Connected';
							document.getElementById('dash-conn-status').textContent = 'Online';
							showScreen('dashboard');
						} else if (!screens.choice.classList.contains('active') && 
								   !screens.qr.classList.contains('active') && 
								   !screens.code.classList.contains('active')) {
							showScreen('phone');
						}
						break;

					case 'pair_qr':
						if (event.data && event.data.code) {
							const wrap = document.getElementById('qr-canvas-wrap');
							wrap.innerHTML = '';
							if (typeof QRCode !== 'undefined' && typeof QRCode.toCanvas === 'function') {
								QRCode.toCanvas(event.data.code, { width: 220, margin: 1 }, (err, canvas) => {
									if (!err) wrap.appendChild(canvas);
								});
							} else if (typeof QRCode === 'function') {
								new QRCode(wrap, { text: event.data.code, width: 220, height: 220 });
							}
							document.getElementById('qr-status-text').textContent = 'Waiting for scan…';
							document.getElementById('qr-dot').classList.add('online');
						}
						break;

					case 'pair_code':
						if (event.data && event.data.code) {
							const code = event.data.code.replace(/-/g, '');
							const display = document.getElementById('pair-code-display');
							display.innerHTML = '<span>' + code.slice(0, 4) + '</span><span>' + code.slice(4) + '</span>';
							document.getElementById('code-status-text').textContent = 'Waiting for confirmation…';
							document.getElementById('code-dot').classList.add('online');
						}
						break;

					case 'pair_error':
						const errEl = document.getElementById('phone-error');
						const errText = document.getElementById('phone-error-text');
						errText.textContent = (event.data && event.data.reason) || 'Pairing error occurred.';
						errEl.classList.add('active');
						showScreen('phone');
						break;

					case 'message':
						addFeedItem(event.data);
						break;
				}
			}

			function addFeedItem(m) {
				const feed = document.getElementById('dash-feed');
				const empty = feed.querySelector('.feed-empty');
				if (empty) empty.remove();

				const item = document.createElement('div');
				item.className = 'feed-item';
				const dateStr = new Date(m.timestamp * 1000).toLocaleTimeString();
				item.innerHTML = '<div class="top"><span>' + (m.pushName || m.sender || m.from) + '</span><span>' + dateStr + '</span></div>' +
								 '<div class="body">' + (m.text || '[Media message]') + '</div>';
				feed.prepend(item);
			}

			// Handlers
			document.getElementById('btn-continue').onclick = () => {
				const phoneInput = document.getElementById('phone-number');
				const val = phoneInput.value.replace(/\D/g, '');
				if (!val) {
					document.getElementById('phone-error-text').textContent = 'Please enter a valid phone number.';
					document.getElementById('phone-error').classList.add('active');
					return;
				}
				userPhone = val;
				document.getElementById('phone-error').classList.remove('active');
				showScreen('choice');
			};

			document.getElementById('choice-qr').onclick = () => {
				showScreen('qr');
				sendAction('request_pair_qr');
			};

			document.getElementById('choice-pair').onclick = () => {
				showScreen('code');
				sendAction('request_pair_code', { phoneNumber: userPhone });
			};

			document.getElementById('choice-back').onclick = () => showScreen('phone');
			document.getElementById('qr-back').onclick = () => showScreen('choice');
			document.getElementById('code-back').onclick = () => showScreen('choice');

			document.getElementById('btn-logout').onclick = () => {
				sendAction('logout');
				showScreen('phone');
			};

			initWS();
		})();
	`

	// Build Page
	doc.Body().Child(
		htmlbuilder.El("script").Attr("src", "https://cdnjs.cloudflare.com/ajax/libs/qrcodejs/1.0.0/qrcode.min.js"),
		screenPhone,
		screenChoice,
		screenQR,
		screenCode,
		screenDashboard,
	)
	doc.Script(scriptBlock)

	return doc.String()
}
