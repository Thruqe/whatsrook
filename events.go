package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/Thruqe/whatsrook/commands"
	"github.com/Thruqe/whatsrook/sender"
	"github.com/Thruqe/whatsrook/updater"
	"github.com/Thruqe/whatsrook/utils"
	"go.mau.fi/whatsmeow/proto/waE2E"
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
			Payload: PairErrorPayload{Reason: v.Error.Error()},
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
		go b.notifyOwnerConnected()

	case *events.Message:
		if pretty, err := json.MarshalIndent(v, "", "  "); err == nil {
			slog.Info("Incoming message event payload (pretty JSON)", "json", string(pretty))
		}

		if commands.HandlePendingAudioReply(context.Background(), b.client, v) {
			return
		}

		if commands.Dispatch(context.Background(), b.client, v) {
			return
		}

		payload := buildIncomingMessagePayload(v)
		b.hub.Broadcast(EventMessage{
			Kind:    EventIncomingMessage,
			Payload: payload,
		})

	case *events.CallOffer:
		slog.Info("call offer received")
		b.hub.Broadcast(EventMessage{
			Kind: EventIncomingCall,
			Payload: IncomingCallPayload{
				CallID:    v.CallID,
				From:      v.CallCreator.String(),
				Timestamp: v.Timestamp,
			},
		})

	case *events.Receipt, *events.PushName, *events.Presence, *events.ChatPresence, *events.AppState, *events.AppStateSyncComplete, *events.Contact, *events.OfflineSyncPreview, *events.OfflineSyncCompleted:
		// Ignore common presence/receipt/keepalive/sync events to avoid debug log clutter

	default:
		slog.Debug("unhandled event", "type", fmt.Sprintf("%T", evt))
	}
}

func buildIncomingMessagePayload(v *events.Message) IncomingMessagePayload {
	text := extractMessageText(v)
	mediaType := getMediaType(v.Message)

	var quotedID string
	var quotedText string

	if ext := v.Message.GetExtendedTextMessage(); ext != nil && ext.GetContextInfo() != nil {
		ci := ext.GetContextInfo()
		quotedID = ci.GetStanzaID()
		if ci.QuotedMessage != nil {
			quotedText = extractTextFromProto(ci.QuotedMessage)
		}
	}

	return IncomingMessagePayload{
		From:       v.Info.Chat.String(),
		Chat:       v.Info.Chat.String(),
		Sender:     v.Info.Sender.String(),
		Text:       text,
		MessageID:  v.Info.ID,
		PushName:   v.Info.PushName,
		Timestamp:  v.Info.Timestamp,
		IsGroup:    v.Info.IsGroup,
		IsFromMe:   v.Info.IsFromMe,
		MediaType:  mediaType,
		QuotedID:   quotedID,
		QuotedText: quotedText,
	}
}

func getMediaType(msg *waE2E.Message) string {
	if msg == nil {
		return ""
	}
	switch {
	case msg.ImageMessage != nil:
		return "image"
	case msg.VideoMessage != nil:
		return "video"
	case msg.AudioMessage != nil:
		return "audio"
	case msg.DocumentMessage != nil:
		return "document"
	case msg.StickerMessage != nil:
		return "sticker"
	case msg.ContactMessage != nil || msg.ContactsArrayMessage != nil:
		return "contact"
	case msg.LocationMessage != nil || msg.LiveLocationMessage != nil:
		return "location"
	default:
		return ""
	}
}

func extractTextFromProto(msg *waE2E.Message) string {
	if msg == nil {
		return ""
	}
	if msg.GetConversation() != "" {
		return msg.GetConversation()
	}
	if ext := msg.GetExtendedTextMessage(); ext != nil {
		return ext.GetText()
	}
	if doc := msg.GetDocumentMessage(); doc != nil {
		return doc.GetCaption()
	}
	if img := msg.GetImageMessage(); img != nil {
		return img.GetCaption()
	}
	if vid := msg.GetVideoMessage(); vid != nil {
		return vid.GetCaption()
	}
	return ""
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

func (b *Bot) notifyOwnerConnected() {
	if b.client == nil || b.client.Store.ID == nil {
		return
	}
	ownerJID := b.client.Store.ID.ToNonAD()

	verStr, err := updater.ReadLocalVersion(updater.VersionFile)
	if err != nil {
		verStr = "unknown"
	}

	meta := utils.GetSystemMetadata(verStr)
	msgText := fmt.Sprintf(
		"WhatsRook Connected Successfully\n\n"+
			"Version: %s\n"+
			"Git Commit: %s\n"+
			"Session: %s\n"+
			"OS/Arch: %s/%s\n"+
			"CPU Cores: %d\n"+
			"Go Runtime: %s",
		meta.Version,
		meta.Commit,
		b.cli.Session,
		meta.OS,
		meta.Arch,
		meta.NumCPU,
		meta.GoVersion,
	)

	formatted := sender.FormatTextResponseRaw(msgText)
	if _, err := b.client.SendMessage(context.Background(), ownerJID, &waE2E.Message{
		Conversation: &formatted,
	}); err != nil {
		slog.Error("failed to send connection metadata notification to owner DM", "err", err)
	} else {
		slog.Info("sent connection metadata notification to owner DM", "owner", ownerJID.String())
	}
}
