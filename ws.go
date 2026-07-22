package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/Thruqe/whatsrook/proto/wsproto"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	googleProto "google.golang.org/protobuf/proto"
)

type Hub struct {
	mu      sync.RWMutex
	clients map[*wsClient]struct{}

	// Inbound control messages from any WS client
	Control chan ControlMessage
}

type wsClient struct {
	conn       *websocket.Conn
	send       chan EventMessage
	isProtobuf bool
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

func (h *Hub) ServeWS(dev bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		subprotocols := []string{"protobuf", "json"}
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			InsecureSkipVerify: dev,
			Subprotocols:       subprotocols,
		})
		if err != nil {
			slog.Error("ws accept failed", "err", err)
			return
		}

		isProtobuf := conn.Subprotocol() == "protobuf"

		c := &wsClient{
			conn:       conn,
			send:       make(chan EventMessage, 64),
			isProtobuf: isProtobuf,
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

			_ = conn.Close(websocket.StatusNormalClosure, "session ended")
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
					if err := conn.Ping(ctx); err != nil {
						cancel()
						return
					}

				case msg, ok := <-c.send:
					if !ok {
						return
					}

					if c.isProtobuf {
						frame := EventMessageToProto(msg)
						data, err := googleProto.Marshal(frame)
						if err != nil {
							slog.Error("failed to marshal proto event frame", "err", err)
							continue
						}
						if err := conn.Write(ctx, websocket.MessageBinary, data); err != nil {
							cancel()
							return
						}
					} else {
						if err := wsjson.Write(ctx, conn, msg); err != nil {
							cancel()
							return
						}
					}
				}
			}
		}()

		// reader loop — supports both JSON text frames and Protobuf binary frames
		for {
			msgType, data, err := conn.Read(ctx)
			if err != nil {
				break
			}

			var ctrl ControlMessage

			if msgType == websocket.MessageBinary || c.isProtobuf {
				var frame wsproto.ControlFrame
				if err := googleProto.Unmarshal(data, &frame); err != nil {
					slog.Warn("bad protobuf control frame", "err", err)
					continue
				}
				pCtrl, err := ControlProtoToMessage(&frame)
				if err != nil {
					slog.Warn("failed to convert proto control frame", "err", err)
					continue
				}
				ctrl = pCtrl
			} else {
				if err := json.Unmarshal(data, &ctrl); err != nil {
					slog.Warn("bad json control frame", "err", err)
					continue
				}
			}

			select {
			case h.Control <- ctrl:
			default:
				slog.Warn("control channel full, dropping message", "id", ctrl.ID)
				select {
				case c.send <- ackEvent(ctrl.ID, false, "server busy"):
				default:
				}
			}
		}

		slog.Info("websocket client disconnected", "subprotocol", conn.Subprotocol())
	}
}
