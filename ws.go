package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"

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
	defer h.mu.RUnlock()
	for c := range h.clients {
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

	c := &wsClient{conn: conn, send: make(chan EventMessage, 64)}

	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()

	ctx, cancel := context.WithCancel(r.Context())
	defer func() {
		cancel()
		err := conn.CloseNow()
		if err != nil {
			fmt.Printf("Failed to close websocket connection: %v", err)
			return
		}
		h.mu.Lock()
		delete(h.clients, c)
		h.mu.Unlock()
	}()

	// writer goroutine
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case msg := <-c.send:
				if err := wsjson.Write(ctx, conn, msg); err != nil {
					cancel()
					return
				}
			}
		}
	}()

	// reader loop
	for {
		var raw map[string]json.RawMessage
		if err := wsjson.Read(ctx, conn, &raw); err != nil {
			break
		}

		var ctrl ControlMessage
		data, _ := json.Marshal(raw)
		if err := json.Unmarshal(data, &ctrl); err != nil {
			ack := ackEvent("unknown", false, fmt.Sprintf("parse error: %s", err))
			_ = wsjson.Write(ctx, conn, ack)
			continue
		}

		// immediate ack
		_ = wsjson.Write(ctx, conn, ackEvent(ctrl.ID, true, ""))

		select {
		case h.Control <- ctrl:
		default:
			slog.Warn("control channel full, dropping message", "id", ctrl.ID)
		}
	}

	slog.Info("websocket client disconnected")
}
