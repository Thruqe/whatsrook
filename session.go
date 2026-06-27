package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

func runSession(ctx context.Context, client *whatsmeow.Client, cli CliArgs, hub *Hub) error {
	client.AddEventHandler(func(evt any) {
		handleWAEvent(evt, cli, hub)
	})

	if client.Store.ID == nil {
		if cli.Pair != "" {
			// Pair code flow
			slog.Info("requesting pair code", "phone", cli.Pair)
			if err := client.Connect(); err != nil {
				return err
			}
			code, err := client.PairPhone(ctx, cli.Pair, true, whatsmeow.PairClientChrome, "Chrome (Linux)")
			if err != nil {
				return fmt.Errorf("pair code failed: %w", err)
			}
			slog.Info("pair code", "code", code)
			fmt.Printf("Enter this code on your phone: %s\n", code)
			hub.Broadcast(EventMessage{
				Kind: EventPairCode,
				Payload: map[string]any{
					"code": code,
				},
			})
		} else {
			// QR flow
			qrChan, _ := client.GetQRChannel(ctx)
			if err := client.Connect(); err != nil {
				return err
			}
			for evt := range qrChan {
				if evt.Event == "code" {
					if cli.QRCode {
						fmt.Println("QR code:", evt.Code)
					}
					hub.Broadcast(EventMessage{
						Kind:    EventPairQR,
						Payload: map[string]any{"code": evt.Code},
					})
				} else {
					slog.Info("qr channel event", "event", evt.Event)
				}
			}
		}
	} else {
		if err := client.Connect(); err != nil {
			return err
		}
	}

	// Control message dispatch loop
	for {
		select {
		case <-ctx.Done():
			return nil
		case ctrl := <-hub.Control:
			handleControl(ctx, client, ctrl)
		}
	}
}

func handleWAEvent(evt any, cli CliArgs, hub *Hub) {
	switch v := evt.(type) {
	case *events.QR:
		// handled via qrChan above, but just in case
		_ = v

	case *events.PairSuccess:
		slog.Info("paired successfully")
		hub.Broadcast(simpleEvent(EventPairSuccess))

	case *events.PairError:
		slog.Warn("pairing failed", "err", v.Error)
		hub.Broadcast(EventMessage{
			Kind:    EventPairError,
			Payload: map[string]any{"reason": v.Error.Error()},
		})

	case *events.LoggedOut:
		slog.Warn("logged out", "reason", v.Reason)
		hub.Broadcast(simpleEvent(EventLoggedOut))

	case *events.Disconnected:
		slog.Info("disconnected")
		hub.Broadcast(simpleEvent(EventDisconnected))

	case *events.Connected:
		slog.Info("connected", "session", cli.Session)
		hub.Broadcast(simpleEvent(EventConnected))

	case *events.Message:
		text := ""
		if v.Message.GetConversation() != "" {
			text = v.Message.GetConversation()
		} else if v.Message.GetExtendedTextMessage() != nil {
			text = v.Message.GetExtendedTextMessage().GetText()
		}
		from := v.Info.Sender.String()
		msgID := v.Info.ID
		slog.Info("message", "from", from, "text", text)
		hub.Broadcast(EventMessage{
			Kind: EventIncomingMessage,
			Payload: map[string]any{
				"from":       from,
				"text":       text,
				"message_id": msgID,
			},
		})

	default:
		slog.Debug("unhandled event", "type", fmt.Sprintf("%T", evt))
	}
}

func handleControl(ctx context.Context, client *whatsmeow.Client, ctrl ControlMessage) {
	switch ctrl.Kind {
	case ControlSendMessage:
		var p SendMessagePayload
		if err := json.Unmarshal(ctrl.Payload, &p); err != nil {
			slog.Warn("bad send_message payload", "err", err)
			return
		}
		jid, err := types.ParseJID(p.To)
		if err != nil {
			slog.Warn("invalid JID", "to", p.To, "err", err)
			return
		}
		var msg waE2E.Message
		if p.QuoteID != nil && p.QuoteSender != nil {
			msg = waE2E.Message{
				ExtendedTextMessage: &waE2E.ExtendedTextMessage{
					Text: proto.String(p.Text),
					ContextInfo: &waE2E.ContextInfo{
						StanzaID:    p.QuoteID,
						Participant: p.QuoteSender,
					},
				},
			}
		} else {
			msg = waE2E.Message{Conversation: proto.String(p.Text)}
		}
		resp, err := client.SendMessage(ctx, jid, &msg)
		if err != nil {
			slog.Error("send failed", "err", err)
		} else {
			slog.Info("sent", "id", resp.ID)
		}

	case ControlSendReaction:
		var p SendReactionPayload
		if err := json.Unmarshal(ctrl.Payload, &p); err != nil {
			slog.Warn("bad send_reaction payload", "err", err)
			return
		}
		jid, err := types.ParseJID(p.To)
		if err != nil {
			slog.Warn("invalid JID", "err", err)
			return
		}
		senderJID := types.EmptyJID
		if p.Sender != nil {
			senderJID, err = types.ParseJID(*p.Sender)
			if err != nil {
				slog.Warn("invalid sender JID", "err", err)
				return
			}
		}
		_, err = client.SendMessage(ctx, jid, client.BuildReaction(jid, senderJID, types.MessageID(p.MessageID), p.Emoji))
		if err != nil {
			slog.Error("reaction failed", "err", err)
		}
	case ControlEditMessage:
		var p EditMessagePayload
		if err := json.Unmarshal(ctrl.Payload, &p); err != nil {
			slog.Warn("bad edit_message payload", "err", err)
			return
		}
		jid, err := types.ParseJID(p.To)
		if err != nil {
			slog.Warn("invalid JID", "err", err)
			return
		}
		_, err = client.SendMessage(ctx, jid, client.BuildEdit(jid, p.MessageID, &waE2E.Message{
			Conversation: proto.String(p.NewText),
		}))
		if err != nil {
			slog.Error("edit failed", "err", err)
		}

	case ControlRevokeMessage:
		var p RevokeMessagePayload
		if err := json.Unmarshal(ctrl.Payload, &p); err != nil {
			slog.Warn("bad revoke_message payload", "err", err)
			return
		}
		jid, err := types.ParseJID(p.To)
		if err != nil {
			slog.Warn("invalid JID", "err", err)
			return
		}
		var revokeMsg *waE2E.Message
		if p.OriginalSender != nil {
			revokeMsg = client.BuildRevoke(jid, types.NewJID(*p.OriginalSender, types.DefaultUserServer), p.MessageID)
		} else {
			revokeMsg = client.BuildRevoke(jid, types.EmptyJID, p.MessageID)
		}
		_, err = client.SendMessage(ctx, jid, revokeMsg)
		if err != nil {
			slog.Error("revoke failed", "err", err)
		}

	case ControlGetStatus:
		slog.Info("status",
			"connected", client.IsConnected(),
			"logged_in", client.IsLoggedIn(),
		)

	case ControlDisconnect:
		slog.Info("disconnect requested")
		client.Disconnect()

	case ControlLogout:
		slog.Info("logout requested")
		if err := client.Logout(ctx); err != nil {
			slog.Error("logout failed", "err", err)
		}
	}
}
