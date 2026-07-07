package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/Thruqe/whatsrook/commands"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waCompanionReg"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

// Bot holds shared state so handleControl can access both client and hub.
type Bot struct {
	client *whatsmeow.Client
	hub    *Hub
	cli    CliArgs
}

func newBot(client *whatsmeow.Client, hub *Hub, cli CliArgs) *Bot {
	return &Bot{client: client, hub: hub, cli: cli}
}

func (b *Bot) run(ctx context.Context) error {
	b.client.AddEventHandler(func(evt any) {
		b.handleWAEvent(evt)
	})

	switch b.cli.Client {
	case ClientAndroid:
		store.DeviceProps.PlatformType = waCompanionReg.DeviceProps_ANDROID_PHONE.Enum()
		store.DeviceProps.Os = new("Android")
	case ClientIos:
		store.DeviceProps.PlatformType = waCompanionReg.DeviceProps_IOS_PHONE.Enum()
		store.DeviceProps.Os = new("iOS")
	default: // ClientChrome
		store.DeviceProps.PlatformType = waCompanionReg.DeviceProps_CHROME.Enum()
		store.DeviceProps.Os = new("Linux")
	}

	if b.client.Store.ID == nil {
		if b.cli.Pair {
			if err := b.runPairCode(ctx); err != nil {
				return err
			}
		} else {
			if err := b.runQR(ctx); err != nil {
				return err
			}
		}
	} else {
		if err := b.client.Connect(); err != nil {
			return err
		}
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case ctrl := <-b.hub.Control:
			ack := b.handleControl(ctx, ctrl)
			b.hub.Broadcast(ack)
		}
	}
}

func (b *Bot) runPairCode(ctx context.Context) error {
	slog.Info("requesting pair code", "phone", b.cli.Session)

	paired := make(chan error, 1)
	b.client.AddEventHandler(func(evt any) {
		switch v := evt.(type) {
		case *events.PairSuccess:
			paired <- nil
		case *events.PairError:
			paired <- fmt.Errorf("pair error: %w", v.Error)
		}
	})

	if err := b.client.Connect(); err != nil {
		return err
	}

	var pairType whatsmeow.PairClientType
	var clientDisplay string

	// FIX: Display name MUST strictly match the "Browser (OS)" template format
	switch b.cli.Client {
	case ClientAndroid:
		pairType = whatsmeow.PairClientAndroid
		clientDisplay = "Chrome (Android)"
	case ClientIos:
		pairType = whatsmeow.PairClientChrome
		clientDisplay = "Chrome (iOS)"
	default:
		pairType = whatsmeow.PairClientChrome
		clientDisplay = "Chrome (Linux)"
	}

	code, err := b.client.PairPhone(ctx, b.cli.Session, true, pairType, clientDisplay)
	if err != nil {
		return fmt.Errorf("pair code failed: %w", err)
	}
	slog.Info("pair code issued", "code", code)
	fmt.Printf("Enter this code on your phone: %s\n", code)
	b.hub.Broadcast(EventMessage{
		Kind:    EventPairCode,
		Payload: map[string]any{"code": code},
	})

	select {
	case err := <-paired:
		if err != nil {
			return err
		}
		slog.Info("paired successfully")
		return nil
	case <-ctx.Done():
		return nil
	}
}

func (b *Bot) runQR(ctx context.Context) error {
	qrChan, _ := b.client.GetQRChannel(ctx)
	if err := b.client.Connect(); err != nil {
		return err
	}
	for evt := range qrChan {
		if evt.Event == "code" {
			if b.cli.QRCode {
				fmt.Println("QR code:", evt.Code)
			}
			b.hub.Broadcast(EventMessage{
				Kind:    EventPairQR,
				Payload: map[string]any{"code": evt.Code},
			})
		} else {
			slog.Info("qr channel event", "event", evt.Event)
		}
	}
	return nil
}

func (b *Bot) handleWAEvent(evt any) {
	switch v := evt.(type) {
	case *events.QR:
		_ = v // handled via qrChan in runQR

	case *events.PairSuccess:
		slog.Info("paired successfully")
		b.hub.Broadcast(simpleEvent(EventPairSuccess))

	case *events.PairError:
		slog.Warn("pairing failed", "err", v.Error)
		b.hub.Broadcast(EventMessage{
			Kind:    EventPairError,
			Payload: map[string]any{"reason": v.Error.Error()},
		})

	case *events.LoggedOut:
		slog.Warn("logged out", "reason", v.Reason)
		b.hub.Broadcast(simpleEvent(EventLoggedOut))

	case *events.Disconnected:
		slog.Info("disconnected")
		b.hub.Broadcast(simpleEvent(EventDisconnected))

	case *events.Connected:
		slog.Info("connected", "session", b.cli.Session)
		b.hub.Broadcast(simpleEvent(EventConnected))

	case *events.Message:
		text := ""
		if v.Message.GetConversation() != "" {
			text = v.Message.GetConversation()
		} else if v.Message.GetExtendedTextMessage() != nil {
			text = v.Message.GetExtendedTextMessage().GetText()
		} else if v.Message.DocumentMessage.GetCaption() != "" {
			text = v.Message.DocumentMessage.GetCaption()
		} else if v.Message.ImageMessage.GetCaption() != "" {
			text = v.Message.ImageMessage.GetCaption()
		} else if v.Message.VideoMessage.GetCaption() != "" {
			text = v.Message.VideoMessage.GetCaption()
		}

		from := v.Info.Sender.String()
		msgID := v.Info.ID

		if commands.Dispatch(context.Background(), b.client, v) {
			return // don't broadcast raw text if it was a command
		}

		b.hub.Broadcast(EventMessage{
			Kind: EventIncomingMessage,
			Payload: map[string]any{
				"from":       from,
				"text":       text,
				"message_id": msgID,
			},
		})
	case *events.CallOffer:
		slog.Info("call offer received")
		b.hub.Broadcast(EventMessage{
			Kind:    EventIncomingCall,
			Payload: map[string]any{"call_id": v.CallID},
		})

	default:
		slog.Debug("unhandled event", "type", fmt.Sprintf("%T", evt))
	}
}

func (b *Bot) handleControl(ctx context.Context, ctrl ControlMessage) EventMessage {
	switch ctrl.Kind {
	case ControlSendMessage:
		var p SendMessagePayload
		if err := json.Unmarshal(ctrl.Payload, &p); err != nil {
			slog.Warn("bad send_message payload", "err", err)
			return ackEvent(ctrl.ID, false, "invalid payload")
		}
		jid, err := types.ParseJID(p.To)
		if err != nil {
			slog.Warn("invalid JID", "to", p.To, "err", err)
			return ackEvent(ctrl.ID, false, "invalid JID: "+err.Error())
		}
		var msg waE2E.Message
		if p.QuoteID != nil && p.QuoteSender != nil {
			msg = waE2E.Message{
				ExtendedTextMessage: &waE2E.ExtendedTextMessage{
					Text: new(p.Text),
					ContextInfo: &waE2E.ContextInfo{
						StanzaID:    p.QuoteID,
						Participant: p.QuoteSender,
					},
				},
			}
		} else {
			msg = waE2E.Message{Conversation: new(p.Text)}
		}
		resp, err := b.client.SendMessage(ctx, jid, &msg)
		if err != nil {
			slog.Error("send failed", "err", err)
			return ackEvent(ctrl.ID, false, err.Error())
		}
		slog.Info("sent", "id", resp.ID)
		return ackEvent(ctrl.ID, true, "")

	case ControlSendReaction:
		var p SendReactionPayload
		if err := json.Unmarshal(ctrl.Payload, &p); err != nil {
			slog.Warn("bad send_reaction payload", "err", err)
			return ackEvent(ctrl.ID, false, "invalid payload")
		}
		jid, err := types.ParseJID(p.To)
		if err != nil {
			slog.Warn("invalid JID", "err", err)
			return ackEvent(ctrl.ID, false, "invalid JID: "+err.Error())
		}
		senderJID := types.EmptyJID
		if p.Sender != nil {
			senderJID, err = types.ParseJID(*p.Sender)
			if err != nil {
				slog.Warn("invalid sender JID", "err", err)
				return ackEvent(ctrl.ID, false, "invalid sender JID: "+err.Error())
			}
		}
		_, err = b.client.SendMessage(ctx, jid, b.client.BuildReaction(jid, senderJID, types.MessageID(p.MessageID), p.Emoji))
		if err != nil {
			slog.Error("reaction failed", "err", err)
			return ackEvent(ctrl.ID, false, err.Error())
		}
		return ackEvent(ctrl.ID, true, "")

	case ControlEditMessage:
		var p EditMessagePayload
		if err := json.Unmarshal(ctrl.Payload, &p); err != nil {
			slog.Warn("bad edit_message payload", "err", err)
			return ackEvent(ctrl.ID, false, "invalid payload")
		}
		jid, err := types.ParseJID(p.To)
		if err != nil {
			slog.Warn("invalid JID", "err", err)
			return ackEvent(ctrl.ID, false, "invalid JID: "+err.Error())
		}
		_, err = b.client.SendMessage(ctx, jid, b.client.BuildEdit(jid, p.MessageID, &waE2E.Message{
			Conversation: new(string),
		}))
		if err != nil {
			slog.Error("edit failed", "err", err)
			return ackEvent(ctrl.ID, false, err.Error())
		}
		return ackEvent(ctrl.ID, true, "")

	case ControlRevokeMessage:
		var p RevokeMessagePayload
		if err := json.Unmarshal(ctrl.Payload, &p); err != nil {
			slog.Warn("bad revoke_message payload", "err", err)
			return ackEvent(ctrl.ID, false, "invalid payload")
		}
		jid, err := types.ParseJID(p.To)
		if err != nil {
			slog.Warn("invalid JID", "err", err)
			return ackEvent(ctrl.ID, false, "invalid JID: "+err.Error())
		}
		var revokeMsg *waE2E.Message
		if p.OriginalSender != nil {
			revokeMsg = b.client.BuildRevoke(jid, types.NewJID(*p.OriginalSender, types.DefaultUserServer), p.MessageID)
		} else {
			revokeMsg = b.client.BuildRevoke(jid, types.EmptyJID, p.MessageID)
		}
		_, err = b.client.SendMessage(ctx, jid, revokeMsg)
		if err != nil {
			slog.Error("revoke failed", "err", err)
			return ackEvent(ctrl.ID, false, err.Error())
		}
		return ackEvent(ctrl.ID, true, "")

	case ControlGetStatus:
		connected := b.client.IsConnected()
		loggedIn := b.client.IsLoggedIn()
		slog.Info("status", "connected", connected, "logged_in", loggedIn)
		return EventMessage{
			Kind: EventStatus,
			ID:   &ctrl.ID,
			Payload: map[string]any{
				"connected": connected,
				"logged_in": loggedIn,
			},
		}

	case ControlDisconnect:
		slog.Info("disconnect requested")
		b.client.Disconnect()
		return ackEvent(ctrl.ID, true, "")

	case ControlLogout:
		slog.Info("logout requested")
		if err := b.client.Logout(ctx); err != nil {
			slog.Error("logout failed", "err", err)
			return ackEvent(ctrl.ID, false, err.Error())
		}
		return ackEvent(ctrl.ID, true, "")

	default:
		slog.Warn("unknown control type", "kind", ctrl.Kind)
		return ackEvent(ctrl.ID, false, "unknown control type")
	}
}
