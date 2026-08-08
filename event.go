package whatsrook

import (
	"context"
	// "encoding/json"
	"fmt"
	"log/slog"
	"time"

	"whatsrook/cli/plugins"
	"whatsrook/cli/updater"
	"whatsrook/messaging"
	"whatsrook/utils"
	"whatsrook/wa-core/store/sqlstore"

	"whatsrook/wa-core/proto/waE2E"
	"whatsrook/wa-core/types/events"
)

func (b *Bot) WAEventHandler(evt any) {
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
		slog.Info("connected", "session", b.cfg.Session)
		b.hub.Broadcast(simpleEvent(EventConnected))
		go b.notifyOwnerConnected()

	case *events.Message:

		// a, _ := json.MarshalIndent(v, "", "  ")
		// fmt.Println(string(a))

		// Skip messages sent before the bot started running
		if b.cfg.SkipOldMessages && v.Info.Timestamp.Before(b.startupTime) {
			return
		}

		if v.Info.Chat.Server == "broadcast" || v.Info.Chat.String() == "status@broadcast" {
			go b.handleLikeStatus(context.Background(), v)
		}

		if v.Info.IsGroup {
			if s, ok := b.client.Store.Identities.(*sqlstore.SQLStore); ok {
				_ = s.RecordParticipantActivity(context.Background(), v.Info.Chat, v.Info.Sender, v.Info.Timestamp)
			}
		}
		if plugins.HandlePendingAudioReply(context.Background(), b.client, v) {
			return
		}
		if plugins.HandlePendingDLReply(context.Background(), b.client, v) {
			return
		}
		if plugins.HandlePendingMenuMediaReply(context.Background(), b.client, v) {
			return
		}
		if plugins.HandlePendingBotCustomizationReply(context.Background(), b.client, v) {
			return
		}

		if plugins.Dispatch(context.Background(), b.client, v) {
			return
		}

		payload := buildIncomingMessagePayload(v)
		b.hub.Broadcast(EventMessage{
			Kind:    EventIncomingMessage,
			Payload: payload,
		})

	case *events.Presence:
		slog.Debug("events: received Presence event", "from", v.From.String(), "unavailable", v.Unavailable, "lastSeen", v.LastSeen)
		plugins.TrackPresence(v.From, !v.Unavailable)

	case *events.ChatPresence:
		slog.Debug("events: received ChatPresence event", "sender", v.Sender.String(), "state", v.State, "media", v.Media)
		if v.Chat.Server == "g.us" && !v.Sender.IsEmpty() {
			if s, ok := b.client.Store.Identities.(*sqlstore.SQLStore); ok {
				_ = s.RecordParticipantActivity(context.Background(), v.Chat, v.Sender, time.Now())
			}
		}
		plugins.TrackPresence(v.Sender, true)

	case *events.Receipt:
		if !v.Sender.IsEmpty() {
			if v.Chat.Server == "g.us" {
				if s, ok := b.client.Store.Identities.(*sqlstore.SQLStore); ok {
					_ = s.RecordParticipantActivity(context.Background(), v.Chat, v.Sender, v.Timestamp)
				}
			}
			plugins.TrackPresence(v.Sender, true)
		}

	case *events.CallOffer:
		slog.Info("call offer received", "from", v.CallCreator.String())
		b.handleAntiCall(context.Background(), v)
		b.hub.Broadcast(EventMessage{
			Kind: EventIncomingCall,
			Payload: IncomingCallPayload{
				CallID:    v.CallID,
				From:      v.CallCreator.String(),
				Timestamp: v.Timestamp,
			},
		})

	case *events.GroupInfo:
		slog.Info("group info update received", "jid", v.JID.String())
		b.handleGroupGreetings(context.Background(), v)
		b.handleGroupEventsNotification(context.Background(), v)

	case *events.PushName, *events.AppState, *events.AppStateSyncComplete, *events.Contact, *events.OfflineSyncPreview, *events.OfflineSyncCompleted, *events.CallAccept, *events.CallPreAccept, *events.CallRelayLatency, *events.CallTerminate, *events.UnknownCallEvent:
		// Ignore low-level call signaling & receipt events to avoid log clutter

	default:
		slog.Debug("unhandled event", "type", fmt.Sprintf("%T", evt))
	}
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
		"Hello @%s 👋\n\n"+
			"WhatsRook Connected\n\n"+
			"Version: %s\n"+
			"Git Commit: %s\n"+
			"Session: %s\n"+
			"OS/Arch: %s/%s\n"+
			"CPU Cores: %d\n"+
			"Go Runtime: %s",
		ownerJID.User,
		meta.Version,
		meta.Commit,
		b.cfg.Session,
		meta.OS,
		meta.Arch,
		meta.NumCPU,
		meta.GoVersion,
	)

	formatted := messaging.FormatTextResponseRaw(msgText)
	if _, err := b.client.SendMessage(context.Background(), ownerJID, &waE2E.Message{
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text: &formatted,
			ContextInfo: &waE2E.ContextInfo{
				MentionedJID: []string{ownerJID.String()},
			},
		},
	}); err != nil {
		slog.Error("failed to send connection metadata notification to owner DM", "err", err)
	} else {
		slog.Info("sent connection metadata notification to owner DM", "owner", ownerJID.String())
	}
}
