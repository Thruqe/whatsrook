package main

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

type Hub struct {
	mu      sync.RWMutex
	clients map[*wsClient]struct{}

	// Inbound control messages from any WS client
	Control chan ControlMessage
}

type wsClient struct {
	conn *websocket.Conn
	send chan EventMessage
}

func newHub() *Hub {
	return &Hub{
		clients: make(map[*wsClient]struct{}),
		Control: make(chan ControlMessage, 64),
	}
}

// Broadcast sends an event to all connected WebSocket clients.
func (h *Hub) Broadcast(evt EventMessage) {
	h.mu.RLock()

	clients := make([]*wsClient, 0, len(h.clients))
	for c := range h.clients {
		clients = append(clients, c)
	}

	h.mu.RUnlock()

	for _, c := range clients {
		select {
		case c.send <- evt:
		default:
			// slow client — drop
		}
	}
}

func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true, // allow any origin in dev
	})
	if err != nil {
		slog.Error("ws accept failed", "err", err)
		return
	}

	c := &wsClient{
		conn: conn,
		send: make(chan EventMessage, 64),
	}

	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()

	ctx, cancel := context.WithCancel(r.Context())

	defer func() {
		cancel()

		h.mu.Lock()

		if _, ok := h.clients[c]; ok {
			delete(h.clients, c)
			close(c.send)
		}

		h.mu.Unlock()

		if err := conn.CloseNow(); err != nil {
			slog.Error(
				"failed to close websocket connection",
				"err",
				err,
			)
		}
	}()

	// single writer goroutine
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()

		for {
			select {

			case <-ctx.Done():
				return

			case <-ticker.C:
				// Send a keep-alive ping so upstream proxies can detect dead connections
				if err := conn.Ping(ctx); err != nil {
					cancel()
					return
				}

			case msg, ok := <-c.send:
				if !ok {
					return
				}

				if err := wsjson.Write(
					ctx,
					conn,
					msg,
				); err != nil {
					cancel()
					return
				}
			}
		}
	}()

	// reader loop
	for {
		var ctrl ControlMessage

		// remove marshal->unmarshal roundtrip
		if err := wsjson.Read(ctx, conn, &ctrl); err != nil {
			break
		}

		// immediate ack through writer channel
		select {
		case c.send <- ackEvent(ctrl.ID, true, ""):
		default:
		}

		select {
		case h.Control <- ctrl:

		default:
			slog.Warn(
				"control channel full, dropping message",
				"id",
				ctrl.ID,
			)

			select {
			case c.send <- ackEvent(
				ctrl.ID,
				false,
				"server busy",
			):
			default:
			}
		}
	}

	slog.Info("websocket client disconnected")
}
