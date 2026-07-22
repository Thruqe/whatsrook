package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/Thruqe/whatsrook/commands"
	"go.mau.fi/whatsmeow/types/events"
)

func (b *Bot) handleWAEvent(evt any) {
	slog.Debug("handleWAEvent received event", "type", fmt.Sprintf("%T", evt))
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
		if pretty, err := json.MarshalIndent(v, "", "  "); err == nil {
			slog.Info("Incoming message event payload (pretty JSON)", "json", string(pretty))
		}

		text := extractMessageText(v)
		from := v.Info.Sender.String()
		msgID := v.Info.ID

		if commands.HandlePendingAudioReply(context.Background(), b.client, v) {
			return
		}

		if commands.Dispatch(context.Background(), b.client, v) {
			return
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

	case *events.Receipt, *events.PushName, *events.Presence, *events.ChatPresence, *events.AppState, *events.AppStateSyncComplete, *events.Contact, *events.OfflineSyncPreview, *events.OfflineSyncCompleted:
		// Ignore common presence/receipt/keepalive/sync events to avoid debug log clutter

	default:
		slog.Debug("unhandled event", "type", fmt.Sprintf("%T", evt))
	}
}

func extractMessageText(v *events.Message) string {
	if v.Message.GetConversation() != "" {
		return v.Message.GetConversation()
	}
	if v.Message.GetExtendedTextMessage() != nil {
		return v.Message.GetExtendedTextMessage().GetText()
	}
	if v.Message.DocumentMessage.GetCaption() != "" {
		return v.Message.DocumentMessage.GetCaption()
	}
	if v.Message.ImageMessage.GetCaption() != "" {
		return v.Message.ImageMessage.GetCaption()
	}
	if v.Message.VideoMessage.GetCaption() != "" {
		return v.Message.VideoMessage.GetCaption()
	}
	return ""
}
