// Package main demonstrates how to connect to WhatsRook over WebSocket,
// receive real-time events, decode binary Protobuf payloads, and send
// control commands (e.g. status requests or logout) using protocol buffers.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"time"

	"whatsrook/proto/wsproto"

	"github.com/coder/websocket"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

func main() {
	wsURL := flag.String("url", "ws://localhost:8080/ws", "WhatsRook WebSocket URL")

	// Config flags
	cfg := Config{}
	flag.BoolVar(&cfg.QRCODE, "qrcode", false, "Enable QR code print / request flags")
	flag.BoolVar(&cfg.PAIR, "pair", false, "Enable phone pair code request flow")
	flag.BoolVar(&cfg.LOGOUT, "logout", false, "Automatically send logout control command after 30 seconds")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	opts := &websocket.DialOptions{
		Subprotocols: []string{"protobuf"},
	}

	log.Println("Connecting to WhatsRook using Protobuf binary subprotocol...")
	conn, _, err := websocket.Dial(ctx, *wsURL, opts)
	if err != nil {
		log.Fatalf("Failed to connect to WhatsRook WS: %v", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "client exiting")

	log.Println("Successfully connected to WhatsRook WebSocket gateway!")

	// Launch binary Protobuf reader loop
	go func() {
		for {
			msgType, data, err := conn.Read(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				log.Printf("Read error: %v", err)
				return
			}

			if msgType != websocket.MessageBinary {
				log.Printf("Ignored non-binary frame type: %v", msgType)
				continue
			}

			handleProtoEvent(data, cfg)
		}
	}()

	// Initial status query control request
	reqID := fmt.Sprintf("req-%d", time.Now().UnixNano())
	sendProtoControlStatus(ctx, conn, reqID)

	// If PAIR flag is passed, send a pairing code request control message
	if cfg.PAIR {
		pairID := fmt.Sprintf("pair-%d", time.Now().UnixNano())
		sendProtoControlPairCode(ctx, conn, pairID)
	} else if cfg.QRCODE {
		qrID := fmt.Sprintf("qr-%d", time.Now().UnixNano())
		sendProtoControlPairQR(ctx, conn, qrID)
	}

	// If LOGOUT config is enabled, schedule automated logout after 30 seconds
	if cfg.LOGOUT {
		log.Println("[Config] LOGOUT=true: Scheduled automated logout control command in 30 seconds...")
		go func() {
			select {
			case <-time.After(30 * time.Second):
				logoutID := fmt.Sprintf("logout-%d", time.Now().UnixNano())
				log.Println("[Config] 30s timer expired — Sending Protobuf LOGOUT control command to WhatsRook...")
				sendProtoLogout(ctx, conn, logoutID)
			case <-ctx.Done():
				return
			}
		}()
	}

	<-ctx.Done()
	log.Println("Exiting client example...")
}

func handleProtoEvent(data []byte, cfg Config) {
	var frame wsproto.EventFrame
	if err := proto.Unmarshal(data, &frame); err != nil {
		log.Printf("[Protobuf] Failed to unmarshal EventFrame: %v", err)
		return
	}

	// Print raw bytes length and header info
	fmt.Printf("\n--- [RECEIVED PROTOBUF EVENT] (%d bytes) ---\n", len(data))
	fmt.Printf("Event Type : %s\n", frame.Type.String())
	if frame.Id != "" {
		fmt.Printf("Request ID : %s\n", frame.Id)
	}

	switch p := frame.Payload.(type) {
	case *wsproto.EventFrame_Message:
		m := p.Message
		push := ""
		if m.PushName != nil {
			push = *m.PushName
		}
		fmt.Printf("Message ID : %s\n", m.MessageId)
		fmt.Printf("From       : %s (%s)\n", m.Sender, push)
		fmt.Printf("Chat       : %s\n", m.Chat)
		fmt.Printf("Text       : %s\n", m.Text)
		if m.MediaType != nil {
			fmt.Printf("Media Type : %s\n", *m.MediaType)
		}
		if m.QuotedId != nil {
			fmt.Printf("Quoted ID  : %s\n", *m.QuotedId)
		}
	case *wsproto.EventFrame_Status:
		s := p.Status
		fmt.Printf("Connected  : %v\n", s.Connected)
		fmt.Printf("Logged In  : %v\n", s.LoggedIn)
		if s.Jid != nil {
			fmt.Printf("Bot JID    : %s\n", *s.Jid)
		}
		if s.PushName != nil {
			fmt.Printf("Push Name  : %s\n", *s.PushName)
		}
	case *wsproto.EventFrame_Ack:
		a := p.Ack
		fmt.Printf("Status     : OK=%v\n", a.Ok)
		if a.Error != nil {
			fmt.Printf("Error      : %s\n", *a.Error)
		}
	case *wsproto.EventFrame_PairCode:
		fmt.Printf("Pair Code  : %s\n", p.PairCode.Code)
		if cfg.PAIR || cfg.QRCODE {
			fmt.Printf("[PAIR CODE] Enter this code on your phone: %s\n", p.PairCode.Code)
		}
	case *wsproto.EventFrame_PairQr:
		fmt.Printf("Pair QR    : %s\n", p.PairQr.Code)
		if cfg.PAIR || cfg.QRCODE {
			fmt.Println("[PAIR QR] Scan this QR code link or use it in your app to link devices.")
		}
	case *wsproto.EventFrame_IncomingCall:
		fmt.Printf("Call ID    : %s\n", p.IncomingCall.CallId)
		fmt.Printf("Caller     : %s\n", p.IncomingCall.From)
	}

	// Full Protobuf text representation
	fmt.Println("Full Frame Details:")
	fmt.Println(protojson.Format(&frame))
	fmt.Println("--------------------------------------------------")
}

func sendProtoControlStatus(ctx context.Context, conn *websocket.Conn, id string) {
	frame := &wsproto.ControlFrame{
		Type: wsproto.ControlType_CONTROL_TYPE_GET_STATUS,
		Id:   id,
		Payload: &wsproto.ControlFrame_GetStatus{
			GetStatus: &wsproto.GetStatusPayload{},
		},
	}
	data, err := proto.Marshal(frame)
	if err != nil {
		log.Printf("[Protobuf] Failed to marshal control message: %v", err)
		return
	}
	log.Printf("[Protobuf] Sending get_status request (ID: %s)...", id)
	if err := conn.Write(ctx, websocket.MessageBinary, data); err != nil {
		log.Printf("[Protobuf] Failed to write binary frame: %v", err)
	}
}

func sendProtoLogout(ctx context.Context, conn *websocket.Conn, id string) {
	frame := &wsproto.ControlFrame{
		Type: wsproto.ControlType_CONTROL_TYPE_LOGOUT,
		Id:   id,
		Payload: &wsproto.ControlFrame_Logout{
			Logout: &wsproto.LogoutPayload{},
		},
	}
	data, err := proto.Marshal(frame)
	if err != nil {
		log.Printf("[Protobuf] Failed to marshal logout control message: %v", err)
		return
	}
	log.Printf("[Protobuf] Sending LOGOUT request (ID: %s)...", id)
	if err := conn.Write(ctx, websocket.MessageBinary, data); err != nil {
		log.Printf("[Protobuf] Failed to write binary frame: %v", err)
	}
}

func sendProtoControlPairCode(ctx context.Context, conn *websocket.Conn, id string) {
	frame := &wsproto.ControlFrame{
		Type: wsproto.ControlType_CONTROL_TYPE_REQUEST_PAIR_CODE,
		Id:   id,
		Payload: &wsproto.ControlFrame_RequestPairCode{
			RequestPairCode: &wsproto.RequestPairCodePayload{},
		},
	}
	data, err := proto.Marshal(frame)
	if err != nil {
		log.Printf("[Protobuf] Failed to marshal request_pair_code control message: %v", err)
		return
	}
	log.Printf("[Protobuf] Sending request_pair_code control command (ID: %s)...", id)
	if err := conn.Write(ctx, websocket.MessageBinary, data); err != nil {
		log.Printf("[Protobuf] Failed to write binary frame: %v", err)
	}
}

func sendProtoControlPairQR(ctx context.Context, conn *websocket.Conn, id string) {
	frame := &wsproto.ControlFrame{
		Type: wsproto.ControlType_CONTROL_TYPE_REQUEST_PAIR_QR,
		Id:   id,
		Payload: &wsproto.ControlFrame_RequestPairQr{
			RequestPairQr: &wsproto.RequestPairQRPayload{},
		},
	}
	data, err := proto.Marshal(frame)
	if err != nil {
		log.Printf("[Protobuf] Failed to marshal request_pair_qr control message: %v", err)
		return
	}
	log.Printf("[Protobuf] Sending request_pair_qr control command (ID: %s)...", id)
	if err := conn.Write(ctx, websocket.MessageBinary, data); err != nil {
		log.Printf("[Protobuf] Failed to write binary frame: %v", err)
	}
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
