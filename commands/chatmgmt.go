package commands

import (
	"fmt"
	"strings"
	"time"

	"github.com/Thruqe/whatsrook/store/sqlstore"
	"go.mau.fi/whatsmeow/appstate"
	waBinary "go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

func init() {
	Register(&Command{
		Name:        "archive",
		Description: "Archive the current chat",
		Category:    "chat",
		IsPublic:    false,
		Handler:     handleArchive,
	})
	Register(&Command{
		Name:        "unarchive",
		Description: "Unarchive the current chat",
		Category:    "chat",
		IsPublic:    false,
		Handler:     handleUnarchive,
	})
	Register(&Command{
		Name:        "pin",
		Description: "Pin the current chat (or pin the replied message)",
		Category:    "chat",
		IsPublic:    true,
		Handler:     handlePin,
	})
	Register(&Command{
		Name:        "unpin",
		Description: "Unpin the current chat (or unpin the replied message)",
		Category:    "chat",
		IsPublic:    true,
		Handler:     handleUnpin,
	})
	Register(&Command{
		Name:        "block",
		Description: "Block the target contact or current private chat JID",
		Category:    "chat",
		IsPublic:    false,
		Handler:     handleBlock,
	})
	Register(&Command{
		Name:        "unblock",
		Description: "Unblock the target contact or current private chat JID",
		Category:    "chat",
		IsPublic:    false,
		Handler:     handleUnblock,
	})
	Register(&Command{
		Name:        "clear",
		Description: "Clear all messages in the current chat",
		Category:    "chat",
		IsPublic:    false,
		Handler:     handleClear,
	})
	Register(&Command{
		Name:        "delete",
		Aliases:     []string{"del", "dlt"},
		Description: "Delete/revoke a message (must reply to the target message)",
		Category:    "chat",
		IsPublic:    true,
		Handler:     handleDelete,
	})
	Register(&Command{
		Name:        "report",
		Description: "Submit a spam report for the target user or replied message to WhatsApp",
		Category:    "chat",
		IsPublic:    false,
		Handler:     handleReport,
	})
}

func handleArchive(ctx *Context) error {
	if !ctx.IsSudo() {
		return ctx.Reply(" Restricted to sudoers only.")
	}
	patch := appstate.BuildArchive(ctx.Chat, true, time.Time{}, nil)
	err := ctx.Client.SendAppState(ctx.Ctx, patch)
	if err != nil {
		return ctx.Reply(" Failed to archive chat: " + err.Error())
	}
	return ctx.Reply(" Chat archived.")
}

func handleUnarchive(ctx *Context) error {
	if !ctx.IsSudo() {
		return ctx.Reply(" Restricted to sudoers only.")
	}
	patch := appstate.BuildArchive(ctx.Chat, false, time.Time{}, nil)
	err := ctx.Client.SendAppState(ctx.Ctx, patch)
	if err != nil {
		return ctx.Reply(" Failed to unarchive chat: " + err.Error())
	}
	return ctx.Reply(" Chat unarchived.")
}

func handlePin(ctx *Context) error {
	ci := ctx.GetContextInfo()
	if ci != nil && ci.StanzaID != nil {
		// Pin message
		isAuthorized := ctx.IsSudo()
		if !isAuthorized && ctx.Chat.Server == "g.us" {
			info, err := ctx.Client.GetGroupInfo(ctx.Ctx, ctx.Chat)
			if err == nil && info != nil {
				if ctx.IsSenderAdmin(info) {
					isAuthorized = true
				}
			}
		}
		if !isAuthorized {
			return ctx.Reply(" Only sudoers or group admins can pin messages.")
		}

		quotedSender, _ := ctx.GetQuotedSender()
		quotedFromMe := false
		if ctx.Client.Store.ID != nil {
			quotedFromMe = (quotedSender.ToNonAD() == ctx.Client.Store.ID.ToNonAD())
		}

		var participantStr *string
		if ctx.Chat.Server == "g.us" {
			participantStr = new(quotedSender.String())
		}
		_ = quotedFromMe

		pinMsg := &waE2E.Message{
			PinInChatMessage: &waE2E.PinInChatMessage{
				Key: &waCommon.MessageKey{
					FromMe:      new(quotedFromMe),
					ID:          ci.StanzaID,
					RemoteJID:   new(ctx.Chat.String()),
					Participant: participantStr,
				},
				Type:              waE2E.PinInChatMessage_PIN_FOR_ALL.Enum(),
				SenderTimestampMS: new(time.Now().UnixMilli()),
			},
		}

		_, err := ctx.Client.SendMessage(ctx.Ctx, ctx.Chat, pinMsg)
		if err != nil {
			return ctx.Reply(" Failed to pin message: " + err.Error())
		}
		return ctx.Reply(" Message pinned.")
	}

	// Pin chat JID (Sudo only)
	if !ctx.IsSudo() {
		return ctx.Reply(" Restricted to sudoers only.")
	}
	patch := appstate.BuildPin(ctx.Chat, true)
	err := ctx.Client.SendAppState(ctx.Ctx, patch)
	if err != nil {
		return ctx.Reply(" Failed to pin chat: " + err.Error())
	}
	return ctx.Reply(" Chat pinned.")
}

func handleUnpin(ctx *Context) error {
	ci := ctx.GetContextInfo()
	if ci != nil && ci.StanzaID != nil {
		// Unpin message
		isAuthorized := ctx.IsSudo()
		if !isAuthorized && ctx.Chat.Server == "g.us" {
			info, err := ctx.Client.GetGroupInfo(ctx.Ctx, ctx.Chat)
			if err == nil && info != nil {
				if ctx.IsSenderAdmin(info) {
					isAuthorized = true
				}
			}
		}
		if !isAuthorized {
			return ctx.Reply(" Only sudoers or group admins can unpin messages.")
		}

		quotedSender, _ := ctx.GetQuotedSender()
		quotedFromMe := false
		if ctx.Client.Store.ID != nil {
			quotedFromMe = (quotedSender.ToNonAD() == ctx.Client.Store.ID.ToNonAD())
		}

		var participantStr *string
		if ctx.Chat.Server == "g.us" {
			participantStr = new(quotedSender.String())
		}
		_ = quotedFromMe

		unpinMsg := &waE2E.Message{
			PinInChatMessage: &waE2E.PinInChatMessage{
				Key: &waCommon.MessageKey{
					FromMe:      new(quotedFromMe),
					ID:          ci.StanzaID,
					RemoteJID:   new(ctx.Chat.String()),
					Participant: participantStr,
				},
				Type:              waE2E.PinInChatMessage_UNPIN_FOR_ALL.Enum(),
				SenderTimestampMS: new(time.Now().UnixMilli()),
			},
		}

		_, err := ctx.Client.SendMessage(ctx.Ctx, ctx.Chat, unpinMsg)
		if err != nil {
			return ctx.Reply(" Failed to unpin message: " + err.Error())
		}
		return ctx.Reply(" Message unpinned.")
	}

	// Unpin chat JID (Sudo only)
	if !ctx.IsSudo() {
		return ctx.Reply(" Restricted to sudoers only.")
	}
	patch := appstate.BuildPin(ctx.Chat, false)
	err := ctx.Client.SendAppState(ctx.Ctx, patch)
	if err != nil {
		return ctx.Reply(" Failed to unpin chat: " + err.Error())
	}
	return ctx.Reply(" Chat unpinned.")
}

func handleBlock(ctx *Context) error {
	if !ctx.IsSudo() {
		return ctx.Reply(" Restricted to sudoers only.")
	}
	target := ctx.Chat
	targets := ctx.GetTargets()
	if len(targets) > 0 {
		target = targets[0]
	}

	if target.Server == "g.us" {
		return ctx.Reply(" Cannot block a group JID. Block commands only apply to users.")
	}

	_, err := ctx.Client.UpdateBlocklist(ctx.Ctx, target, events.BlocklistChangeActionBlock)
	if err != nil {
		return ctx.Reply(" Failed to block user: " + err.Error())
	}
	return ctx.Reply(fmt.Sprintf(" Blocked @%s.", target.User))
}

func handleUnblock(ctx *Context) error {
	if !ctx.IsSudo() {
		return ctx.Reply(" Restricted to sudoers only.")
	}
	target := ctx.Chat
	targets := ctx.GetTargets()
	if len(targets) > 0 {
		target = targets[0]
	}

	if target.Server == "g.us" {
		return ctx.Reply(" Cannot unblock a group. Unblock commands only apply to users.")
	}

	_, err := ctx.Client.UpdateBlocklist(ctx.Ctx, target, events.BlocklistChangeActionUnblock)
	if err != nil {
		return ctx.Reply(" Failed to unblock user: " + err.Error())
	}
	return ctx.Reply(fmt.Sprintf(" Unblocked @%s.", target.User))
}

func handleClear(ctx *Context) error {
	if !ctx.IsSudo() {
		return ctx.Reply(" Restricted to sudoers only.")
	}
	patch := appstate.BuildDeleteChat(ctx.Chat, time.Now(), nil, true)
	err := ctx.Client.SendAppState(ctx.Ctx, patch)
	if err != nil {
		return ctx.Reply(" Failed to clear chat: " + err.Error())
	}
	return ctx.Reply(" Chat messages cleared.")
}

func handleDelete(ctx *Context) error {
	ci := ctx.GetContextInfo()
	if ci == nil || ci.StanzaID == nil {
		return ctx.Reply(" Reply to the message you want to delete.")
	}

	targetID := *ci.StanzaID

	isAuthorized := ctx.IsSudo()
	if !isAuthorized && ctx.Chat.Server == "g.us" {
		info, err := ctx.Client.GetGroupInfo(ctx.Ctx, ctx.Chat)
		if err == nil && info != nil {
			if ctx.IsSenderAdmin(info) {
				isAuthorized = true
			}
		}
	}

	if !isAuthorized {
		return ctx.Reply(" Only sudoers or group admins can delete messages.")
	}

	quotedSender, ok := ctx.GetQuotedSender()
	var revokeSender types.JID
	if ok {
		revokeSender = quotedSender
	} else {
		revokeSender = types.EmptyJID
	}

	revokeMsg := ctx.Client.BuildRevoke(ctx.Chat, revokeSender, targetID)
	_, err := ctx.Client.SendMessage(ctx.Ctx, ctx.Chat, revokeMsg)
	if err != nil {
		return ctx.Reply(" Failed to delete message: " + err.Error())
	}
	return nil
}

func isJIDSudo(ctx *Context, jid types.JID) bool {
	if ctx.Client.Store.ID != nil {
		if ctx.IsSameUser(jid, *ctx.Client.Store.ID) {
			return true
		}
	}
	s, ok := ctx.Client.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return false
	}
	raw, err := s.GetSetting(ctx.Ctx, "sudoers")
	if err != nil || raw == "" {
		return false
	}
	for _, sudoerStr := range strings.Fields(raw) {
		sudoerJID, err := types.ParseJID(sudoerStr)
		if err == nil {
			if ctx.IsSameUser(jid, sudoerJID) {
				return true
			}
		}
	}
	return false
}

func handleReport(ctx *Context) error {
	if !ctx.IsSudo() {
		return ctx.Reply(" Restricted to sudoers only.")
	}

	targetJID := ctx.Chat
	targets := ctx.GetTargets()
	if len(targets) > 0 {
		targetJID = targets[0]
	}

	var messageChild []waBinary.Node
	var spamFlow = "ContactInfo"

	ci := ctx.GetContextInfo()
	if ci != nil && ci.StanzaID != nil {
		quotedSender, _ := ctx.GetQuotedSender()
		if !quotedSender.IsEmpty() {
			targetJID = quotedSender
		}

		spamFlow = "MessageMenu"
		messageChild = []waBinary.Node{
			{
				Tag: "message",
				Attrs: waBinary.Attrs{
					"id": *ci.StanzaID,
					"t":  fmt.Sprintf("%d", time.Now().Unix()),
				},
			},
		}
	}

	// SENSITIVE DATA/AUTHORIZATION SECURITY CHECK:
	// Do not allow reporting the bot or its sudo users.
	if isJIDSudo(ctx, targetJID) {
		return ctx.Reply(" Cannot report the bot or any of its sudo users.")
	}

	// Parse iteration count (e.g. "report 2x" or "report 5")
	count := 1
	for _, arg := range ctx.Args {
		trimmed := strings.ToLower(strings.TrimSpace(arg))
		if strings.HasSuffix(trimmed, "x") {
			numPart := strings.TrimSuffix(trimmed, "x")
			var val int
			if _, err := fmt.Sscan(numPart, &val); err == nil && val > 0 {
				count = val
				break
			}
		} else {
			var val int
			if _, err := fmt.Sscan(trimmed, &val); err == nil && val > 0 {
				count = val
				break
			}
		}
	}
	if count > 20 {
		count = 20
	}

	spamListAttrs := waBinary.Attrs{
		"spam_flow": spamFlow,
	}
	if targetJID.Server == "g.us" {
		spamListAttrs["jid"] = targetJID.String()
		spamListAttrs["subject"] = "Group Spam"
		if spamFlow == "ContactInfo" {
			spamListAttrs["spam_flow"] = "GroupInfoReport"
		}
	} else {
		spamListAttrs["jid"] = targetJID.String()
	}

	// Send the report stanzas in a loop
	for i := 0; i < count; i++ {
		//nolint:staticcheck
		reqID := ctx.Client.DangerousInternals().GenerateRequestID()

		iqNode := waBinary.Node{
			Tag: "iq",
			Attrs: waBinary.Attrs{
				"id":    reqID,
				"to":    types.ServerJID.String(),
				"type":  "set",
				"xmlns": "spam",
			},
			Content: []waBinary.Node{
				{
					Tag:     "spam_list",
					Attrs:   spamListAttrs,
					Content: messageChild,
				},
			},
		}

		//nolint:staticcheck
		_, err := ctx.Client.DangerousInternals().SendNodeAndGetData(ctx.Ctx, iqNode)
		if err != nil {
			return ctx.Reply(fmt.Sprintf(" Failed to submit spam report on iteration %d: %s", i+1, err.Error()))
		}

		if count > 1 && i < count-1 {
			time.Sleep(200 * time.Millisecond)
		}
	}

	// Format success output
	if targetJID.Server == "g.us" {
		groupName := targetJID.String()
		info, err := ctx.Client.GetGroupInfo(ctx.Ctx, targetJID)
		if err == nil && info != nil {
			groupName = info.GroupName.Name
		}
		if count > 1 {
			return ctx.Reply(fmt.Sprintf(" Reported %s for spam to whatsapp %dx.", groupName, count))
		}
		return ctx.Reply(fmt.Sprintf(" Reported %s for spam to whatsapp.", groupName))
	}

	resolvedJID, username := ctx.ResolveMention(targetJID)
	if count > 1 {
		return ctx.ReplyWithMentions(fmt.Sprintf(" Reported @%s for spam to whatsapp %dx.", username, count), []types.JID{resolvedJID})
	}
	return ctx.ReplyWithMentions(fmt.Sprintf(" Reported @%s for spam to whatsapp.", username), []types.JID{resolvedJID})
}
