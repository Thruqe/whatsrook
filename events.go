// WhatsApp event handler – dispatches incoming messages to commands and
// broadcasts them to WebSocket clients.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"whatsrook/commands"
	"whatsrook/ember"
	"whatsrook/sender"
	"whatsrook/store/sqlstore"
	"whatsrook/updater"
	"whatsrook/utils"

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

	case *events.Receipt, *events.PushName, *events.Presence, *events.ChatPresence, *events.AppState, *events.AppStateSyncComplete, *events.Contact, *events.OfflineSyncPreview, *events.OfflineSyncCompleted, *events.CallAccept, *events.CallPreAccept, *events.CallRelayLatency, *events.CallTerminate, *events.UnknownCallEvent:
		// Ignore low-level call signaling & presence/receipt events to avoid log clutter

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

func (b *Bot) handleAntiCall(ctx context.Context, v *events.CallOffer) {
	if b.client == nil || v == nil {
		return
	}
	s, ok := b.client.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return
	}

	status, _ := s.GetSetting(ctx, "anticall_status")
	if status != "on" {
		return
	}

	callerJID := v.CallCreator
	callerNum := callerJID.User

	contactsOnly, _ := s.GetSetting(ctx, "anticall_contacts_only")
	allowedCC, _ := s.GetSetting(ctx, "anticall_allowed_cc")

	reject := false

	if contactsOnly == "true" {
		contact, err := b.client.Store.Contacts.GetContact(ctx, callerJID)
		if err != nil || (!contact.Found || (contact.FirstName == "" && contact.FullName == "")) {
			reject = true
		}
	}

	if !reject && allowedCC != "" {
		codes := strings.Split(allowedCC, ",")
		matched := false
		for _, cc := range codes {
			cc = strings.TrimSpace(strings.TrimPrefix(cc, "+"))
			if cc != "" && strings.HasPrefix(callerNum, cc) {
				matched = true
				break
			}
		}
		if !matched {
			reject = true
		}
	}

	if !reject && contactsOnly != "true" && allowedCC == "" {
		reject = true
	}

	if reject {
		slog.Warn("anticall: rejecting call offer", "from", callerJID.String(), "call_id", v.CallID)
		_ = b.client.RejectCall(ctx, callerJID, v.CallID)

		warnKey := "anticall_warn:" + callerJID.String()
		rawWarn, _ := s.GetSetting(ctx, warnKey)
		warnCount, _ := strconv.Atoi(rawWarn)
		warnCount++
		_ = s.PutSetting(ctx, warnKey, strconv.Itoa(warnCount))

		rawMax, _ := s.GetSetting(ctx, "anticall_max_warn")
		maxWarn, _ := strconv.Atoi(rawMax)
		if maxWarn <= 0 {
			maxWarn = 3
		}

		if warnCount >= maxWarn {
			_, _ = b.client.UpdateBlocklist(ctx, callerJID, events.BlocklistChangeActionBlock)
			slog.Warn("anticall: caller blocked after reaching max warnings", "from", callerJID.String(), "warn_count", warnCount)
			warnText := fmt.Sprintf("Call rejected. You have reached the maximum warning threshold (%d/%d) and have been blocked.", warnCount, maxWarn)
			formatted := sender.FormatTextResponseRaw(warnText)
			_, _ = b.client.SendMessage(ctx, callerJID, &waE2E.Message{Conversation: &formatted})
		} else {
			warnText := fmt.Sprintf("Call rejected. Warning %d/%d. Continued calls will result in being blocked.", warnCount, maxWarn)
			formatted := sender.FormatTextResponseRaw(warnText)
			_, _ = b.client.SendMessage(ctx, callerJID, &waE2E.Message{Conversation: &formatted})
		}
	}
}

func (b *Bot) handleGroupGreetings(ctx context.Context, g *events.GroupInfo) {
	if b.client == nil || g == nil {
		return
	}
	s, ok := b.client.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return
	}

	chatKey := g.JID.String()

	// Process joins (Welcome)
	if len(g.Join) > 0 {
		status, _ := s.GetSetting(ctx, "welcome_status:"+chatKey)
		if status == "on" {
			tag, _ := s.GetSetting(ctx, "welcome_tag:"+chatKey)
			descOpt, _ := s.GetSetting(ctx, "welcome_desc:"+chatKey)
			customMsg, _ := s.GetSetting(ctx, "welcome_msg:"+chatKey)
			mediaURL, _ := s.GetSetting(ctx, "welcome_media:"+chatKey)

			info, err := b.client.GetGroupInfo(ctx, g.JID)
			groupName := "the group"
			groupDesc := ""
			memberCount := 0
			if err == nil && info != nil {
				groupName = info.Name
				groupDesc = info.Topic
				memberCount = len(info.Participants)
			}

			for _, participant := range g.Join {
				userTag := "@" + participant.User
				body := customMsg
				if body == "" {
					body = "Welcome " + userTag + " to " + groupName
				} else {
					body = strings.ReplaceAll(body, "{user}", userTag)
					body = strings.ReplaceAll(body, "{group}", groupName)
					body = strings.ReplaceAll(body, "{desc}", groupDesc)
					body = strings.ReplaceAll(body, "{members}", strconv.Itoa(memberCount))
				}

				if descOpt == "on" && groupDesc != "" && !strings.Contains(customMsg, "{desc}") {
					body += "\n\nGroup Description:\n" + groupDesc
				}

				formatted := sender.FormatTextResponseRaw(body)
				var mentions []string
				if tag == "on" {
					mentions = append(mentions, participant.String())
				}

				msg := &waE2E.Message{
					ExtendedTextMessage: &waE2E.ExtendedTextMessage{
						Text: &formatted,
						ContextInfo: &waE2E.ContextInfo{
							MentionedJID: mentions,
						},
					},
				}

				if mediaURL != "" {
					_ = sender.SendResult(ctx, b.client, g.JID, &ember.Data{
						Medias: []ember.Media{{URL: mediaURL, Type: "video"}},
					})
				} else {
					_, _ = b.client.SendMessage(ctx, g.JID, msg)
				}
			}
		}
	}

	// Process leaves (Goodbye)
	if len(g.Leave) > 0 {
		status, _ := s.GetSetting(ctx, "goodbye_status:"+chatKey)
		if status == "on" {
			tag, _ := s.GetSetting(ctx, "goodbye_tag:"+chatKey)
			descOpt, _ := s.GetSetting(ctx, "goodbye_desc:"+chatKey)
			customMsg, _ := s.GetSetting(ctx, "goodbye_msg:"+chatKey)
			mediaURL, _ := s.GetSetting(ctx, "goodbye_media:"+chatKey)

			info, err := b.client.GetGroupInfo(ctx, g.JID)
			groupName := "the group"
			groupDesc := ""
			memberCount := 0
			if err == nil && info != nil {
				groupName = info.Name
				groupDesc = info.Topic
				memberCount = len(info.Participants)
			}

			for _, participant := range g.Leave {
				// Check if participant left voluntarily vs kicked out by another admin
				if g.Sender != nil && !g.Sender.IsEmpty() && *g.Sender != participant {
					continue
				}

				userTag := "@" + participant.User
				body := customMsg
				if body == "" {
					body = "Goodbye " + userTag + " from " + groupName
				} else {
					body = strings.ReplaceAll(body, "{user}", userTag)
					body = strings.ReplaceAll(body, "{group}", groupName)
					body = strings.ReplaceAll(body, "{desc}", groupDesc)
					body = strings.ReplaceAll(body, "{members}", strconv.Itoa(memberCount))
				}

				if descOpt == "on" && groupDesc != "" && !strings.Contains(customMsg, "{desc}") {
					body += "\n\nGroup Description:\n" + groupDesc
				}

				formatted := sender.FormatTextResponseRaw(body)
				var mentions []string
				if tag == "on" {
					mentions = append(mentions, participant.String())
				}

				msg := &waE2E.Message{
					ExtendedTextMessage: &waE2E.ExtendedTextMessage{
						Text: &formatted,
						ContextInfo: &waE2E.ContextInfo{
							MentionedJID: mentions,
						},
					},
				}

				if mediaURL != "" {
					_ = sender.SendResult(ctx, b.client, g.JID, &ember.Data{
						Medias: []ember.Media{{URL: mediaURL, Type: "video"}},
					})
				} else {
					_, _ = b.client.SendMessage(ctx, g.JID, msg)
				}
			}
		}
	}
}
