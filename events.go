// WhatsApp event handler – dispatches incoming messages to commands and
// broadcasts them to WebSocket clients.
package main

import (
	"context"
	// "encoding/json"
	"fmt"
	"log/slog"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"whatsrook/plugins"
	"whatsrook/sender"
	"whatsrook/store/sqlstore"
	"whatsrook/updater"
	"whatsrook/utils"

	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
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

		// a, _ := json.MarshalIndent(v, "", "  ")
		// fmt.Println(string(a))

		// Skip messages sent before the bot started running
		if b.cli.SkipOldMessages && v.Info.Timestamp.Before(b.startupTime) {
			slog.Debug("skipping old message", "timestamp", v.Info.Timestamp, "startup", b.startupTime)
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
		if commands.HandlePendingAudioReply(context.Background(), b.client, v) {
			return
		}
		if commands.HandlePendingDLReply(context.Background(), b.client, v) {
			return
		}
		if commands.HandlePendingMenuMediaReply(context.Background(), b.client, v) {
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

	case *events.Presence:
		slog.Debug("events: received Presence event", "from", v.From.String(), "unavailable", v.Unavailable, "lastSeen", v.LastSeen)
		commands.TrackPresence(v.From, !v.Unavailable)

	case *events.ChatPresence:
		slog.Debug("events: received ChatPresence event", "sender", v.Sender.String(), "state", v.State, "media", v.Media)
		if v.Chat.Server == "g.us" && !v.Sender.IsEmpty() {
			if s, ok := b.client.Store.Identities.(*sqlstore.SQLStore); ok {
				_ = s.RecordParticipantActivity(context.Background(), v.Chat, v.Sender, time.Now())
			}
		}
		commands.TrackPresence(v.Sender, true)

	case *events.Receipt:
		slog.Debug("events: received Receipt event", "sender", v.Sender.String(), "type", v.Type)
		if !v.Sender.IsEmpty() {
			if v.Chat.Server == "g.us" {
				if s, ok := b.client.Store.Identities.(*sqlstore.SQLStore); ok {
					_ = s.RecordParticipantActivity(context.Background(), v.Chat, v.Sender, v.Timestamp)
				}
			}
			commands.TrackPresence(v.Sender, true)
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

func buildIncomingMessagePayload(v *events.Message) IncomingMessagePayload {
	text := utils.ExtractMessageText(v)
	mediaType := utils.GetMediaType(v.Message)

	var quotedID string
	var quotedText string

	if ext := v.Message.GetExtendedTextMessage(); ext != nil && ext.GetContextInfo() != nil {
		ci := ext.GetContextInfo()
		quotedID = ci.GetStanzaID()
		if ci.QuotedMessage != nil {
			quotedText = utils.ExtractTextFromProto(ci.QuotedMessage)
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
			"WhatsRook Connected Successfully!\n\n"+
			"Version: %s\n"+
			"Git Commit: %s\n"+
			"Session: %s\n"+
			"OS/Arch: %s/%s\n"+
			"CPU Cores: %d\n"+
			"Go Runtime: %s",
		ownerJID.User,
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

func (b *Bot) handleAutoAcceptCall(ctx context.Context, v *events.CallOffer) {
	if b.client == nil || v == nil {
		return
	}
	commands.HandleAutoAcceptIncomingCall(ctx, b.client, v)
}

func (b *Bot) handleAntiCall(ctx context.Context, v *events.CallOffer) {
	if b.client == nil || v == nil {
		return
	}
	s, ok := b.client.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return
	}

	autoAcceptStatus, _ := s.GetSetting(ctx, commands.AutoAcceptCallSettingKey)
	if autoAcceptStatus == "on" {
		slog.Debug("anticall: skipping reject because autoacceptcall is enabled", "call_id", v.CallID)
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

			info, err := b.client.GetGroupInfo(ctx, g.JID)
			groupName := "the group"
			groupDesc := ""
			memberCount := 0
			adminCount := 0
			ownerStr := ""
			ownerJIDStr := ""
			createdAtStr := ""
			groupJIDStr := g.JID.String()

			if err == nil && info != nil {
				if info.Name != "" {
					groupName = info.Name
				}
				groupDesc = info.Topic
				memberCount = len(info.Participants)
				for _, p := range info.Participants {
					if p.IsAdmin || p.IsSuperAdmin {
						adminCount++
					}
				}
				if !info.OwnerJID.IsEmpty() {
					ownerJIDStr = info.OwnerJID.String()
					_, ownerName := sender.ResolveMentionRaw(ctx, b.client, info.OwnerJID)
					ownerStr = "@" + ownerName
				}
				if !info.GroupCreated.IsZero() {
					createdAtStr = info.GroupCreated.Format("2006-01-02")
				}
			}

			for _, participant := range g.Join {
				resolvedJID, username := sender.ResolveMentionRaw(ctx, b.client, participant)
				userTag := "@" + username
				body := customMsg
				if body == "" {
					body = "Welcome " + userTag + " to " + groupName
				} else {
					body = strings.ReplaceAll(body, "{user}", userTag)
					body = strings.ReplaceAll(body, "{user_id}", participant.User)
					body = strings.ReplaceAll(body, "{phone}", participant.User)
					body = strings.ReplaceAll(body, "{user_jid}", participant.String())

					body = strings.ReplaceAll(body, "{group}", groupName)
					body = strings.ReplaceAll(body, "{name}", groupName)
					body = strings.ReplaceAll(body, "{group_jid}", groupJIDStr)
					body = strings.ReplaceAll(body, "{jid}", groupJIDStr)

					body = strings.ReplaceAll(body, "{desc}", groupDesc)
					body = strings.ReplaceAll(body, "{topic}", groupDesc)

					body = strings.ReplaceAll(body, "{members}", strconv.Itoa(memberCount))
					body = strings.ReplaceAll(body, "{count}", strconv.Itoa(memberCount))
					body = strings.ReplaceAll(body, "{admins}", strconv.Itoa(adminCount))
					body = strings.ReplaceAll(body, "{admin_count}", strconv.Itoa(adminCount))

					body = strings.ReplaceAll(body, "{owner}", ownerStr)
					body = strings.ReplaceAll(body, "{creator}", ownerStr)

					body = strings.ReplaceAll(body, "{created_at}", createdAtStr)
				}

				if descOpt == "on" && groupDesc != "" && !strings.Contains(customMsg, "{desc}") && !strings.Contains(customMsg, "{topic}") {
					body += "\n\nGroup Description:\n" + groupDesc
				}

				formatted := sender.FormatTextResponseRaw(body)
				var mentions []string
				if tag == "on" {
					mentions = append(mentions, resolvedJID.String())
				}
				if ownerJIDStr != "" && (strings.Contains(customMsg, "{owner}") || strings.Contains(customMsg, "{creator}")) {
					mentions = append(mentions, ownerJIDStr)
				}

				msg := &waE2E.Message{
					ExtendedTextMessage: &waE2E.ExtendedTextMessage{
						Text: &formatted,
						ContextInfo: &waE2E.ContextInfo{
							MentionedJID: mentions,
						},
					},
				}

				_, _ = b.client.SendMessage(ctx, g.JID, msg)
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

			info, err := b.client.GetGroupInfo(ctx, g.JID)
			groupName := "the group"
			groupDesc := ""
			memberCount := 0
			adminCount := 0
			ownerStr := ""
			ownerJIDStr := ""
			createdAtStr := ""
			groupJIDStr := g.JID.String()

			if err == nil && info != nil {
				if info.Name != "" {
					groupName = info.Name
				}
				groupDesc = info.Topic
				memberCount = len(info.Participants)
				for _, p := range info.Participants {
					if p.IsAdmin || p.IsSuperAdmin {
						adminCount++
					}
				}
				if !info.OwnerJID.IsEmpty() {
					ownerJIDStr = info.OwnerJID.String()
					_, ownerName := sender.ResolveMentionRaw(ctx, b.client, info.OwnerJID)
					ownerStr = "@" + ownerName
				}
				if !info.GroupCreated.IsZero() {
					createdAtStr = info.GroupCreated.Format("2006-01-02")
				}
			}

			for _, participant := range g.Leave {
				// Check if participant left voluntarily vs kicked out by another admin
				if g.Sender != nil && !g.Sender.IsEmpty() && *g.Sender != participant {
					continue
				}

				resolvedJID, username := sender.ResolveMentionRaw(ctx, b.client, participant)
				userTag := "@" + username
				body := customMsg
				if body == "" {
					body = "Goodbye " + userTag + " from " + groupName
				} else {
					body = strings.ReplaceAll(body, "{user}", userTag)
					body = strings.ReplaceAll(body, "{user_id}", participant.User)
					body = strings.ReplaceAll(body, "{phone}", participant.User)
					body = strings.ReplaceAll(body, "{user_jid}", participant.String())

					body = strings.ReplaceAll(body, "{group}", groupName)
					body = strings.ReplaceAll(body, "{name}", groupName)
					body = strings.ReplaceAll(body, "{group_jid}", groupJIDStr)
					body = strings.ReplaceAll(body, "{jid}", groupJIDStr)

					body = strings.ReplaceAll(body, "{desc}", groupDesc)
					body = strings.ReplaceAll(body, "{topic}", groupDesc)

					body = strings.ReplaceAll(body, "{members}", strconv.Itoa(memberCount))
					body = strings.ReplaceAll(body, "{count}", strconv.Itoa(memberCount))
					body = strings.ReplaceAll(body, "{admins}", strconv.Itoa(adminCount))
					body = strings.ReplaceAll(body, "{admin_count}", strconv.Itoa(adminCount))

					body = strings.ReplaceAll(body, "{owner}", ownerStr)
					body = strings.ReplaceAll(body, "{creator}", ownerStr)

					body = strings.ReplaceAll(body, "{created_at}", createdAtStr)
				}

				if descOpt == "on" && groupDesc != "" && !strings.Contains(customMsg, "{desc}") && !strings.Contains(customMsg, "{topic}") {
					body += "\n\nGroup Description:\n" + groupDesc
				}

				formatted := sender.FormatTextResponseRaw(body)
				var mentions []string
				if tag == "on" {
					mentions = append(mentions, resolvedJID.String())
				}
				if ownerJIDStr != "" && (strings.Contains(customMsg, "{owner}") || strings.Contains(customMsg, "{creator}")) {
					mentions = append(mentions, ownerJIDStr)
				}

				msg := &waE2E.Message{
					ExtendedTextMessage: &waE2E.ExtendedTextMessage{
						Text: &formatted,
						ContextInfo: &waE2E.ContextInfo{
							MentionedJID: mentions,
						},
					},
				}

				_, _ = b.client.SendMessage(ctx, g.JID, msg)
			}
		}
	}
}

func (b *Bot) handleGroupEventsNotification(ctx context.Context, g *events.GroupInfo) {
	if b.client == nil || g == nil {
		return
	}
	s, ok := b.client.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return
	}

	chatKey := g.JID.String()
	status, _ := s.GetSetting(ctx, "events_status:"+chatKey)
	if status != "on" {
		return
	}

	var actorTag string
	var actorJID *types.JID
	if g.Sender != nil && !g.Sender.IsEmpty() {
		actorJID = g.Sender
		_, actorName := sender.ResolveMentionRaw(ctx, b.client, *g.Sender)
		actorTag = " by @" + actorName
	}

	// 1. Group Subject / Name Changed
	if g.Name != nil && g.Name.Name != "" {
		msgText := fmt.Sprintf("*Group Event*: Group name changed to *%s*%s.", g.Name.Name, actorTag)
		b.sendGroupEventMessage(ctx, g.JID, msgText, actorJID)
	}

	// 2. Group Description / Topic Changed
	if g.Topic != nil && g.Topic.Topic != "" {
		msgText := fmt.Sprintf("*Group Event*: Group description updated%s:\n%s", actorTag, g.Topic.Topic)
		b.sendGroupEventMessage(ctx, g.JID, msgText, actorJID)
	}

	// 3. Announce Mute / Unmute
	if g.Announce != nil {
		if g.Announce.IsAnnounce {
			msgText := fmt.Sprintf("*Group Event*: Group settings updated%s. Only admins can send messages now.", actorTag)
			b.sendGroupEventMessage(ctx, g.JID, msgText, actorJID)
		} else {
			msgText := fmt.Sprintf("*Group Event*: Group settings updated%s. All members can send messages now.", actorTag)
			b.sendGroupEventMessage(ctx, g.JID, msgText, actorJID)
		}
	}

	// 4. Locked / Unlocked
	if g.Locked != nil {
		if g.Locked.IsLocked {
			msgText := fmt.Sprintf("*Group Event*: Group settings locked%s. Only admins can edit group info.", actorTag)
			b.sendGroupEventMessage(ctx, g.JID, msgText, actorJID)
		} else {
			msgText := fmt.Sprintf("*Group Event*: Group settings unlocked%s. All members can edit group info.", actorTag)
			b.sendGroupEventMessage(ctx, g.JID, msgText, actorJID)
		}
	}

	// 5. Admin Promotions
	if len(g.Promote) > 0 {
		for _, userJID := range g.Promote {
			resolvedJID, username := sender.ResolveMentionRaw(ctx, b.client, userJID)
			msgText := fmt.Sprintf("*Group Event*: @%s was promoted to Group Admin%s!", username, actorTag)
			b.sendGroupEventMessageWithMentions(ctx, g.JID, msgText, []types.JID{resolvedJID})
		}
	}

	// 6. Admin Demotions
	if len(g.Demote) > 0 {
		for _, userJID := range g.Demote {
			resolvedJID, username := sender.ResolveMentionRaw(ctx, b.client, userJID)
			msgText := fmt.Sprintf("*Group Event*: @%s was demoted from Group Admin%s.", username, actorTag)
			b.sendGroupEventMessageWithMentions(ctx, g.JID, msgText, []types.JID{resolvedJID})
		}
	}
}

func (b *Bot) handleLikeStatus(ctx context.Context, v *events.Message) {
	if b.client == nil || v == nil {
		return
	}
	s, ok := b.client.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return
	}

	status, _ := s.GetSetting(ctx, "likestatus_status")
	if status != "on" {
		return
	}

	loveEmojis := []string{"❤️", "💕", "💖", "💗", "💓", "💞", "💘", "💌", "🥰", "😍"}
	emoji := loveEmojis[rand.Intn(len(loveEmojis))]

	senderJID := v.Info.Sender
	if senderJID.IsEmpty() {
		senderJID = v.Info.Chat
	}

	reaction := &waE2E.Message{
		ReactionMessage: &waE2E.ReactionMessage{
			Key: &waCommon.MessageKey{
				RemoteJID:   new(v.Info.Chat.String()),
				FromMe:      new(v.Info.IsFromMe),
				ID:          new(v.Info.ID),
				Participant: new(senderJID.String()),
			},
			Text:              new(emoji),
			SenderTimestampMS: new(time.Now().UnixMilli()),
		},
	}

	_, err := b.client.SendMessage(ctx, v.Info.Chat, reaction)
	if err != nil {
		slog.Error("failed to react to status broadcast", "err", err)
	} else {
		slog.Debug("liked status broadcast", "emoji", emoji, "sender", senderJID.String())
	}
}

func (b *Bot) sendGroupEventMessage(ctx context.Context, chatJID types.JID, text string, actor *types.JID) {
	var mentions []types.JID
	if actor != nil && !actor.IsEmpty() {
		mentions = append(mentions, *actor)
	}
	b.sendGroupEventMessageWithMentions(ctx, chatJID, text, mentions)
}

func (b *Bot) sendGroupEventMessageWithMentions(ctx context.Context, chatJID types.JID, text string, targetMentions []types.JID) {
	formatted := sender.FormatTextResponseRaw(text)
	var mentions []string
	for _, m := range targetMentions {
		if !m.IsEmpty() {
			mentions = append(mentions, m.String())
		}
	}

	msg := &waE2E.Message{
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text: &formatted,
			ContextInfo: &waE2E.ContextInfo{
				MentionedJID: mentions,
			},
		},
	}
	_, _ = b.client.SendMessage(ctx, chatJID, msg)
}
