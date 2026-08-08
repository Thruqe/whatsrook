// Group management commands – invite, kick, promote, demote, tag all, etc.
package plugins

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"whatsrook/wa-core/store/sqlstore"

	"whatsrook/wa-core"
	waBinary "whatsrook/wa-core/binary"
	"whatsrook/wa-core/proto/waE2E"
	"whatsrook/wa-core/types"
	"whatsrook/wa-core/types/events"
)

func init() {
	Register(&Command{
		Name:        "tagall",
		Aliases:     []string{"everyone"},
		Description: "Mention everyone in the group",
		Category:    "group",
		GroupOnly:   true,
		IsPublic:    true,
		Handler:     handleTagAll,
	})
	Register(&Command{
		Name:        "kick",
		Description: "Remove a member from the group (reply, tag, or number)",
		Category:    "group",
		GroupOnly:   true,
		IsPublic:    true,
		Handler:     handleKick,
	})
	Register(&Command{
		Name:        "add",
		Description: "Add a member to the group (phone number/JID)",
		Category:    "group",
		GroupOnly:   true,
		IsPublic:    true,
		Handler:     handleAdd,
	})
	Register(&Command{
		Name:        "promote",
		Description: "Promote a member to admin (reply, tag, or number)",
		Category:    "group",
		GroupOnly:   true,
		IsPublic:    true,
		Handler:     handlePromote,
	})
	Register(&Command{
		Name:        "demote",
		Description: "Demote a member from admin (reply, tag, or number)",
		Category:    "group",
		GroupOnly:   true,
		IsPublic:    true,
		Handler:     handleDemote,
	})
	Register(&Command{
		Name:        "group",
		Description: "Manage group settings (open, close, lock, unlock)",
		Category:    "group",
		GroupOnly:   true,
		IsPublic:    true,
		Handler:     handleGroup,
	})
	Register(&Command{
		Name:        "antilink",
		Description: "Enable or disable anti-link protection (on/off)",
		Category:    "group",
		GroupOnly:   true,
		IsPublic:    true,
		Handler:     handleAntiLink,
	})
	Register(&Command{
		Name:        "antiword",
		Description: "Manage banned words (add [word], del [word], list)",
		Category:    "group",
		GroupOnly:   true,
		IsPublic:    true,
		Handler:     handleAntiWord,
	})
	Register(&Command{
		Name:        "gstats",
		Description: "Provide statistics on the most active group participants",
		Category:    "group",
		GroupOnly:   true,
		IsPublic:    true,
		Handler:     handleGStats,
	})
	Register(&Command{
		Name:        "poll",
		Aliases:     []string{"lockpoll"},
		Description: "Create a poll with single or multiple choice selection buttons",
		Category:    "group",
		GroupOnly:   true,
		IsPublic:    true,
		Handler:     handlePoll,
	})
	Register(&Command{
		Name:        "invite",
		Description: "Get the group invite link",
		Category:    "group",
		GroupOnly:   true,
		IsPublic:    true,
		Handler:     handleInvite,
	})
	Register(&Command{
		Name:        "listonline",
		Aliases:     []string{"online", "onlines", "list-online"},
		Description: "List online participants in the current group",
		Category:    "group",
		GroupOnly:   true,
		IsPublic:    true,
		Handler:     handleListOnline,
	})
	Register(&Command{
		Name:        "kickall",
		Description: "Remove all participants from the group except the bot and sudoers",
		Category:    "group",
		GroupOnly:   true,
		IsPublic:    false,
		Handler:     handleKickAll,
	})
	Register(&Command{
		Name:        "community",
		Aliases:     []string{"listgroups", "groupslist", "allgroups"},
		Description: "List community groups or joined groups with their invite links",
		Category:    "group",
		IsPublic:    true,
		Handler:     handleCommunity,
	})
	Register(&Command{
		Name:        "leave",
		Aliases:     []string{"left"},
		Description: "Leave the current group with interactive confirmation",
		Category:    "group",
		GroupOnly:   true,
		IsPublic:    true,
		Handler:     handleLeave,
	})
	Register(&Command{
		Name:        "join",
		Aliases:     []string{"joingroup"},
		Description: "Join a group using a group URL or group invite message",
		Category:    "group",
		IsPublic:    true,
		Handler:     handleJoin,
	})
}

func handleTagAll(ctx *Context) error {
	if ctx.Chat.Server != "g.us" {
		return ctx.Reply("This command can only be used in a group.")
	}
	info, err := ctx.Client.GetGroupInfo(ctx.Ctx, ctx.Chat)
	if err != nil {
		return ctx.Reply(fmt.Sprintf("Failed to get group info: %v", err))
	}
	if !ctx.IsSenderAdmin(info) {
		return ctx.Reply("Only group admins can tag everyone.")
	}

	var sb strings.Builder
	sb.WriteString("@all")
	if ctx.RawArgs != "" {
		sb.WriteString("\nMessage: *")
		sb.WriteString(ctx.RawArgs)
		sb.WriteString("*")
	}

	return ctx.ReplyWithGroupMention(sb.String())
}

func handleKick(ctx *Context) error {
	if ctx.Chat.Server != "g.us" {
		return ctx.Reply("This command can only be used in a group.")
	}
	info, err := ctx.Client.GetGroupInfo(ctx.Ctx, ctx.Chat)
	if err != nil {
		return ctx.Reply(fmt.Sprintf(" Failed to get group info: %v", err))
	}
	if !ctx.IsSenderAdmin(info) {
		return ctx.Reply("Only group admins can kick members.")
	}
	if !ctx.AmIAdmin(info) {
		return ctx.Reply("The bot must be an admin to kick members.")
	}

	targets := ctx.GetTargets()
	if len(targets) == 0 {
		p := ctx.GetPrefix()
		return ctx.Reply(fmt.Sprintf("Usage:\n- %skick @user\n- %skick 1234567890\n- Reply to a user's message with %skick", p, p, p))
	}

	loader := ctx.StartLoader("Removing participant...")
	defer loader.Delete()

	var kicked []string
	var kickedJIDs []types.JID
	for _, target := range targets {
		_, err := ctx.Client.UpdateGroupParticipants(ctx.Ctx, ctx.Chat, []types.JID{target}, whatsmeow.ParticipantChangeRemove)
		resolvedJID, username := ctx.ResolveMention(target)
		if err != nil {
			_ = ctx.ReplyWithMentions(fmt.Sprintf("Failed to kick @%s: %v", username, err), []types.JID{resolvedJID})
		} else {
			kicked = append(kicked, "@"+username)
			kickedJIDs = append(kickedJIDs, resolvedJID)
		}
	}

	if len(kicked) > 0 {
		return ctx.ReplyWithMentions(fmt.Sprintf("Kicked: %s", strings.Join(kicked, ", ")), kickedJIDs)
	}
	return nil
}

func handleAdd(ctx *Context) error {
	if ctx.Chat.Server != "g.us" {
		return ctx.Reply("This command can only be used in a group.")
	}
	info, err := ctx.Client.GetGroupInfo(ctx.Ctx, ctx.Chat)
	if err != nil {
		return ctx.Reply(fmt.Sprintf(" Failed to get group info: %v", err))
	}
	if !ctx.IsSenderAdmin(info) {
		return ctx.Reply("Only group admins can add members.")
	}
	if !ctx.AmIAdmin(info) {
		return ctx.Reply("The bot must be an admin to add members.")
	}

	targets := ctx.GetTargets()
	if len(targets) == 0 {
		p := ctx.GetPrefix()
		return ctx.Reply(fmt.Sprintf("Usage:\n- %sadd 1234567890\n- %sadd 1234567890 9876543210", p, p))
	}

	loader := ctx.StartLoader("Adding participant...")
	defer loader.Delete()

	var added []string
	var addedJIDs []types.JID
	for _, target := range targets {
		_, err := ctx.Client.UpdateGroupParticipants(ctx.Ctx, ctx.Chat, []types.JID{target}, whatsmeow.ParticipantChangeAdd)
		resolvedJID, username := ctx.ResolveMention(target)
		if err != nil {
			_ = ctx.ReplyWithMentions(fmt.Sprintf("Failed to add @%s: %v", username, err), []types.JID{resolvedJID})
		} else {
			added = append(added, "@"+username)
			addedJIDs = append(addedJIDs, resolvedJID)
		}
	}

	if len(added) > 0 {
		return ctx.ReplyWithMentions(fmt.Sprintf("Added: %s", strings.Join(added, ", ")), addedJIDs)
	}
	return nil
}

func handlePromote(ctx *Context) error {
	if ctx.Chat.Server != "g.us" {
		return ctx.Reply("This command can only be used in a group.")
	}
	info, err := ctx.Client.GetGroupInfo(ctx.Ctx, ctx.Chat)
	if err != nil {
		return ctx.Reply(fmt.Sprintf(" Failed to get group info: %v", err))
	}
	if !ctx.IsSenderAdmin(info) {
		return ctx.Reply("Only group admins can promote members.")
	}
	if !ctx.AmIAdmin(info) {
		return ctx.Reply("The bot must be an admin to promote members.")
	}

	targets := ctx.GetTargets()
	if len(targets) == 0 {
		p := ctx.GetPrefix()
		return ctx.Reply(fmt.Sprintf("Usage:\n- %spromote @user\n- %spromote 1234567890\n- Reply to a user's message with %spromote", p, p, p))
	}

	loader := ctx.StartLoader("Promoting participant...")
	defer loader.Delete()

	var promoted []string
	var promotedJIDs []types.JID
	for _, target := range targets {
		_, err := ctx.Client.UpdateGroupParticipants(ctx.Ctx, ctx.Chat, []types.JID{target}, whatsmeow.ParticipantChangePromote)
		resolvedJID, username := ctx.ResolveMention(target)
		if err != nil {
			_ = ctx.ReplyWithMentions(fmt.Sprintf("Failed to promote @%s: %v", username, err), []types.JID{resolvedJID})
		} else {
			promoted = append(promoted, "@"+username)
			promotedJIDs = append(promotedJIDs, resolvedJID)
		}
	}

	if len(promoted) > 0 {
		return ctx.ReplyWithMentions(fmt.Sprintf("Promoted: %s", strings.Join(promoted, ", ")), promotedJIDs)
	}
	return nil
}

func handleDemote(ctx *Context) error {
	if ctx.Chat.Server != "g.us" {
		return ctx.Reply("This command can only be used in a group.")
	}
	info, err := ctx.Client.GetGroupInfo(ctx.Ctx, ctx.Chat)
	if err != nil {
		return ctx.Reply(fmt.Sprintf(" Failed to get group info: %v", err))
	}
	if !ctx.IsSenderAdmin(info) {
		return ctx.Reply("Only group admins can demote members.")
	}
	if !ctx.AmIAdmin(info) {
		return ctx.Reply("The bot must be an admin to demote members.")
	}

	targets := ctx.GetTargets()
	if len(targets) == 0 {
		p := ctx.GetPrefix()
		return ctx.Reply(fmt.Sprintf("Usage:\n- %sdemote @user\n- %sdemote 1234567890\n- Reply to a user's message with %sdemote", p, p, p))
	}

	loader := ctx.StartLoader("Demoting participant...")
	defer loader.Delete()

	var demoted []string
	var demotedJIDs []types.JID
	for _, target := range targets {
		_, err := ctx.Client.UpdateGroupParticipants(ctx.Ctx, ctx.Chat, []types.JID{target}, whatsmeow.ParticipantChangeDemote)
		resolvedJID, username := ctx.ResolveMention(target)
		if err != nil {
			_ = ctx.ReplyWithMentions(fmt.Sprintf("Failed to demote @%s: %v", username, err), []types.JID{resolvedJID})
		} else {
			demoted = append(demoted, "@"+username)
			demotedJIDs = append(demotedJIDs, resolvedJID)
		}
	}

	if len(demoted) > 0 {
		return ctx.ReplyWithMentions(fmt.Sprintf("Demoted: %s", strings.Join(demoted, ", ")), demotedJIDs)
	}
	return nil
}

func handleGroup(ctx *Context) error {
	if ctx.Chat.Server != "g.us" {
		return ctx.Reply("This command can only be used in a group.")
	}
	info, err := ctx.Client.GetGroupInfo(ctx.Ctx, ctx.Chat)
	if err != nil {
		return ctx.Reply(fmt.Sprintf(" Failed to get group info: %v", err))
	}
	if !ctx.IsSenderAdmin(info) {
		return ctx.Reply("Only group admins can change group settings.")
	}
	if !ctx.AmIAdmin(info) {
		return ctx.Reply("The bot must be an admin to change group settings.")
	}

	if len(ctx.Args) == 0 {
		p := ctx.GetPrefix()
		return ctx.Reply(fmt.Sprintf("Usage:\n- %sgroup open\n- %sgroup close\n- %sgroup lock\n- %sgroup unlock", p, p, p, p))
	}

	action := strings.ToLower(ctx.Args[0])
	switch action {
	case "open":
		err = ctx.Client.SetGroupAnnounce(ctx.Ctx, ctx.Chat, false)
		if err != nil {
			return ctx.Reply(fmt.Sprintf("Failed to open group: %v", err))
		}
		return ctx.Reply("Group opened. Everyone can send messages.")
	case "close":
		err = ctx.Client.SetGroupAnnounce(ctx.Ctx, ctx.Chat, true)
		if err != nil {
			return ctx.Reply(fmt.Sprintf("Failed to close group: %v", err))
		}
		return ctx.Reply("Group closed. Only admins can send messages.")
	case "lock":
		err = ctx.Client.SetGroupLocked(ctx.Ctx, ctx.Chat, true)
		if err != nil {
			return ctx.Reply(fmt.Sprintf("Failed to lock group: %v", err))
		}
		return ctx.Reply("Group locked. Only admins can edit group settings.")
	case "unlock":
		err = ctx.Client.SetGroupLocked(ctx.Ctx, ctx.Chat, false)
		if err != nil {
			return ctx.Reply(fmt.Sprintf("Failed to unlock group: %v", err))
		}
		return ctx.Reply("Group unlocked. Everyone can edit group settings.")
	default:
		return ctx.Reply("Invalid action. Usage: group <open|close|lock|unlock>")
	}
}

func handleAntiLink(ctx *Context) error {
	info, err := ctx.Client.GetGroupInfo(ctx.Ctx, ctx.Chat)
	if err != nil {
		return ctx.Reply(fmt.Sprintf("Failed to get group info: %v", err))
	}
	if !ctx.IsSenderAdmin(info) {
		return ctx.Reply("Only group admins can change anti-link settings.")
	}

	s, ok := ctx.Client.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return ctx.Reply("Settings store unavailable.")
	}

	groupName := info.Name
	if groupName == "" {
		groupName = ctx.Chat.String()
	}

	chatKey := ctx.Chat.String()
	statusKey := "antilink:" + chatKey
	modeKey := "antilink_mode:" + chatKey
	customKey := "antilink_custom:" + chatKey

	args := ctx.Args
	sub := ""
	if len(args) > 0 {
		sub = strings.ToLower(args[0])
	}

	p := ctx.GetPrefix()

	switch sub {
	case "on", "enable", "activate":
		_ = s.PutSetting(ctx.Ctx, statusKey, "on")
		return sendAntiLinkMenu(ctx, s, "Anti-link protection has been activated for this group.")

	case "off", "disable", "deactivate":
		_ = s.PutSetting(ctx.Ctx, statusKey, "off")
		return sendAntiLinkMenu(ctx, s, "Anti-link protection has been deactivated for this group.")

	case "toggle":
		curr, _ := s.GetSetting(ctx.Ctx, statusKey)
		if curr == "on" {
			_ = s.PutSetting(ctx.Ctx, statusKey, "off")
			return sendAntiLinkMenu(ctx, s, "Anti-link protection has been deactivated for this group.")
		}
		_ = s.PutSetting(ctx.Ctx, statusKey, "on")
		return sendAntiLinkMenu(ctx, s, "Anti-link protection has been activated for this group.")

	case "mode", "customize":
		bodyText := fmt.Sprintf("╭━━━〔 ANTILINK CUSTOMIZE 〕━━━\n│ Group : %s\n╰━━━━━━━━━━━━━━━━━━━━━━━━━━\n\nChoose Anti-Link Protection Mode:\n\n1. *Default Links*: Block all web links (http://, https://, www, .com, etc.)\n2. *Custom URLs*: Block specific domain patterns separated by comma (e.g. `chat.whatsapp.com, t.me`)", groupName)
		buttons := []struct{ ID, Text string }{
			{ID: p + "antilink default", Text: "Default Links"},
			{ID: p + "antilink custom", Text: "Custom URLs"},
		}
		return sendInteractiveButtons(ctx, bodyText, fmt.Sprintf("%s AntiLink Settings", ctx.GetBotName()), buttons)

	case "default":
		_ = s.PutSetting(ctx.Ctx, modeKey, "default")
		_ = s.PutSetting(ctx.Ctx, statusKey, "on")
		bodyText := fmt.Sprintf("╭━━━〔 ANTILINK MODE SET 〕━━━\n│ Group : %s\n│ Mode  : DEFAULT (ALL LINKS)\n│ Status: ACTIVE\n╰━━━━━━━━━━━━━━━━━━━━━━━━━━\n\nAnti-link will now block all web links sent in this group!", groupName)
		buttons := []struct{ ID, Text string }{
			{ID: p + "antilink off", Text: "Deactivate"},
			{ID: p + "antilink mode", Text: "Customize Mode"},
		}
		return sendInteractiveButtons(ctx, bodyText, fmt.Sprintf("%s AntiLink Settings", ctx.GetBotName()), buttons)

	case "custom", "set":
		customInput := ""
		if len(args) > 1 {
			customInput = strings.Join(args[1:], " ")
		} else if len(args) == 1 && sub != "custom" {
			customInput = args[0]
		}

		customInput = strings.TrimSpace(customInput)
		if customInput == "" || customInput == "custom" {
			_ = s.PutSetting(ctx.Ctx, modeKey, "custom")
			_ = s.PutSetting(ctx.Ctx, statusKey, "on")
			currCustom, _ := s.GetSetting(ctx.Ctx, customKey)
			if currCustom == "" {
				currCustom = "chat.whatsapp.com"
				_ = s.PutSetting(ctx.Ctx, customKey, currCustom)
			}
			bodyText := fmt.Sprintf("╭━━━〔 ANTILINK CUSTOM MODE 〕━━━\n│ Group   : %s\n│ Mode    : CUSTOM DOMAINS\n│ Status  : ACTIVE\n│ Blocked : %s\n╰━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\nTo update custom domains, send:\n`%santilink set domain1, domain2`\n\nExample:\n`%santilink set chat.whatsapp.com, t.me, instagram.com`", groupName, currCustom, p, p)
			buttons := []struct{ ID, Text string }{
				{ID: p + "antilink default", Text: "Default Links"},
				{ID: p + "antilink off", Text: "Deactivate"},
			}
			return sendInteractiveButtons(ctx, bodyText, fmt.Sprintf("%s AntiLink Settings", ctx.GetBotName()), buttons)
		}

		rawParts := strings.Split(customInput, ",")
		var cleaned []string
		for _, part := range rawParts {
			part = strings.TrimSpace(strings.ToLower(part))
			if part != "" {
				cleaned = append(cleaned, part)
			}
		}
		if len(cleaned) == 0 {
			return ctx.Reply("Please specify at least one valid domain pattern separated by comma. Example: `chat.whatsapp.com, t.me`")
		}

		newCustomStr := strings.Join(cleaned, ", ")
		_ = s.PutSetting(ctx.Ctx, customKey, newCustomStr)
		_ = s.PutSetting(ctx.Ctx, modeKey, "custom")
		_ = s.PutSetting(ctx.Ctx, statusKey, "on")

		bodyText := fmt.Sprintf("╭━━━〔 ANTILINK CUSTOMIZED 〕━━━\n│ Group   : %s\n│ Mode    : CUSTOM DOMAINS\n│ Status  : ACTIVE\n│ Blocked : %s\n╰━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\nAnti-link will now block messages containing these custom domain patterns!", groupName, newCustomStr)
		buttons := []struct{ ID, Text string }{
			{ID: p + "antilink off", Text: "Deactivate"},
			{ID: p + "antilink mode", Text: "Customize Mode"},
		}
		return sendInteractiveButtons(ctx, bodyText, fmt.Sprintf("%s AntiLink Settings", ctx.GetBotName()), buttons)

	case "action", "act":
		if len(args) > 1 {
			act := strings.ToLower(args[1])
			if act != "delete" && act != "kick" && act != "warn" {
				return ctx.Reply("Invalid action. Options: delete, kick, warn")
			}
			_ = s.PutSetting(ctx.Ctx, "antilink_action:"+chatKey, act)
			return sendAntiLinkMenu(ctx, s, fmt.Sprintf("Anti-link action mode updated to *%s*.", strings.ToUpper(act)))
		}
		bodyText := fmt.Sprintf("╭━━━〔 ANTILINK ACTION MODE 〕━━━\n│ Group : %s\n╰━━━━━━━━━━━━━━━━━━━━━━━━━━\n\nChoose what happens when a non-admin participant sends a link:\n\n1. *Delete*: Delete message only\n2. *Kick*: Delete message & kick participant\n3. *Warn*: Issue warning (default 3 max). Kick upon reaching threshold", groupName)
		buttons := []struct{ ID, Text string }{
			{ID: p + "antilink action delete", Text: "Delete Only"},
			{ID: p + "antilink action kick", Text: "Kick User"},
			{ID: p + "antilink action warn", Text: "Warn User"},
		}
		return sendInteractiveButtons(ctx, bodyText, fmt.Sprintf("%s AntiLink Action", ctx.GetBotName()), buttons)

	case "setwarn", "maxwarn":
		if len(args) < 2 {
			return ctx.Reply("Please specify warning limit. Example: `antilink setwarn 5`")
		}
		cnt, err := strconv.Atoi(args[1])
		if err != nil || cnt <= 0 {
			return ctx.Reply("Invalid warning limit. Must be a positive integer.")
		}
		_ = s.PutSetting(ctx.Ctx, "antilink_maxwarn:"+chatKey, strconv.Itoa(cnt))
		_ = s.PutSetting(ctx.Ctx, "antilink_action:"+chatKey, "warn")
		return sendAntiLinkMenu(ctx, s, fmt.Sprintf("Anti-link warning limit set to *%d*. Action mode switched to WARN.", cnt))

	default:
		return sendAntiLinkMenu(ctx, s, "")
	}
}

func sendAntiLinkMenu(ctx *Context, s *sqlstore.SQLStore, note string) error {
	chatKey := ctx.Chat.String()
	groupName := chatKey
	info, err := ctx.Client.GetGroupInfo(ctx.Ctx, ctx.Chat)
	if err == nil && info != nil && info.Name != "" {
		groupName = info.Name
	}
	status, _ := s.GetSetting(ctx.Ctx, "antilink:"+chatKey)
	if status == "" {
		status = "off"
	}
	mode, _ := s.GetSetting(ctx.Ctx, "antilink_mode:"+chatKey)
	if mode == "" {
		mode = "default"
	}
	action, _ := s.GetSetting(ctx.Ctx, "antilink_action:"+chatKey)
	if action == "" {
		action = "delete"
	}
	actionDisplay := strings.ToUpper(action)
	if action == "warn" {
		maxWarn, _ := s.GetSetting(ctx.Ctx, "antilink_maxwarn:"+chatKey)
		if maxWarn == "" {
			maxWarn = "3"
		}
		actionDisplay = fmt.Sprintf("WARN (Max: %s)", maxWarn)
	}

	custom, _ := s.GetSetting(ctx.Ctx, "antilink_custom:"+chatKey)

	p := ctx.GetPrefix()
	var sb strings.Builder
	fmt.Fprintf(&sb, "╭━━━〔 ANTILINK CONFIGURATION 〕━━━\n│ Group  : %s\n│ Status : %s\n│ Mode   : %s\n│ Action : %s\n", groupName, strings.ToUpper(status), strings.ToUpper(mode), actionDisplay)
	if mode == "custom" && custom != "" {
		fmt.Fprintf(&sb, "│ Blocked: %s\n", custom)
	}
	sb.WriteString("╰━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

	if note != "" {
		sb.WriteString(note)
		sb.WriteString("\n\n")
	}

	sb.WriteString("Options:\n")
	fmt.Fprintf(&sb, "• `%santilink mode` - Switch between Default Links & Custom URLs\n", p)
	fmt.Fprintf(&sb, "• `%santilink action <delete|kick|warn>` - Set action mode\n", p)
	fmt.Fprintf(&sb, "• `%santilink setwarn 3` - Customize max warnings\n", p)

	var toggleBtn struct{ ID, Text string }
	if status == "on" {
		toggleBtn = struct{ ID, Text string }{ID: p + "antilink off", Text: "Deactivate"}
	} else {
		toggleBtn = struct{ ID, Text string }{ID: p + "antilink on", Text: "Activate"}
	}

	buttons := []struct{ ID, Text string }{
		toggleBtn,
		{ID: p + "antilink action", Text: "Action Mode"},
		{ID: p + "antilink mode", Text: "Customize"},
	}

	return sendInteractiveButtons(ctx, strings.TrimSpace(sb.String()), fmt.Sprintf("%s AntiLink Moderation", ctx.GetBotName()), buttons)
}

func handleAntiWord(ctx *Context) error {
	info, err := ctx.Client.GetGroupInfo(ctx.Ctx, ctx.Chat)
	if err != nil {
		return ctx.Reply(fmt.Sprintf("Failed to get group info: %v", err))
	}
	if !ctx.IsSenderAdmin(info) {
		return ctx.Reply("Only group admins can change anti-word settings.")
	}

	s, ok := ctx.Client.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return ctx.Reply("Settings store unavailable.")
	}

	groupName := info.Name
	if groupName == "" {
		groupName = ctx.Chat.String()
	}

	chatKey := ctx.Chat.String()
	settingKey := "antiword:" + chatKey
	raw, _ := s.GetSetting(ctx.Ctx, settingKey)
	words := strings.Fields(raw)

	args := ctx.Args
	sub := ""
	if len(args) > 0 {
		sub = strings.ToLower(args[0])
	}

	p := ctx.GetPrefix()

	switch sub {
	case "on", "enable", "activate":
		_ = s.PutSetting(ctx.Ctx, "antiword_status:"+chatKey, "on")
		return sendAntiWordMenu(ctx, s, "Anti-word protection activated.")

	case "off", "disable", "deactivate":
		_ = s.PutSetting(ctx.Ctx, "antiword_status:"+chatKey, "off")
		return sendAntiWordMenu(ctx, s, "Anti-word protection deactivated.")

	case "action", "act":
		if len(args) > 1 {
			act := strings.ToLower(args[1])
			if act != "delete" && act != "kick" && act != "warn" {
				return ctx.Reply("Invalid action. Options: delete, kick, warn")
			}
			_ = s.PutSetting(ctx.Ctx, "antiword_action:"+chatKey, act)
			return sendAntiWordMenu(ctx, s, fmt.Sprintf("Anti-word action mode set to *%s*.", strings.ToUpper(act)))
		}
		bodyText := fmt.Sprintf("╭━━━〔 ANTIWORD ACTION MODE 〕━━━\n│ Group : %s\n╰━━━━━━━━━━━━━━━━━━━━━━━━━━\n\nChoose what happens when a non-admin participant sends a banned word:\n\n1. *Delete*: Delete message only\n2. *Kick*: Delete message & kick participant\n3. *Warn*: Issue warning (default 3 max). Kick upon reaching threshold", groupName)
		buttons := []struct{ ID, Text string }{
			{ID: p + "antiword action delete", Text: "Delete Only"},
			{ID: p + "antiword action kick", Text: "Kick User"},
			{ID: p + "antiword action warn", Text: "Warn User"},
		}
		return sendInteractiveButtons(ctx, bodyText, fmt.Sprintf("%s AntiWord Action", ctx.GetBotName()), buttons)

	case "setwarn", "maxwarn":
		if len(args) < 2 {
			return ctx.Reply("Please specify warning limit. Example: `antiword setwarn 5`")
		}
		cnt, err := strconv.Atoi(args[1])
		if err != nil || cnt <= 0 {
			return ctx.Reply("Invalid warning limit. Must be a positive integer.")
		}
		_ = s.PutSetting(ctx.Ctx, "antiword_maxwarn:"+chatKey, strconv.Itoa(cnt))
		_ = s.PutSetting(ctx.Ctx, "antiword_action:"+chatKey, "warn")
		return sendAntiWordMenu(ctx, s, fmt.Sprintf("Anti-word warning limit set to *%d*. Action mode switched to WARN.", cnt))

	case "add":
		if len(args) < 2 {
			return ctx.Reply("Please specify the word to add.")
		}
		wordToAdd := strings.ToLower(args[1])
		if slices.Contains(words, wordToAdd) {
			return ctx.Reply(fmt.Sprintf("Word %q is already banned.", wordToAdd))
		}
		words = append(words, wordToAdd)
		_ = s.PutSetting(ctx.Ctx, settingKey, strings.Join(words, " "))
		_ = s.PutSetting(ctx.Ctx, "antiword_status:"+chatKey, "on")
		return sendAntiWordMenu(ctx, s, fmt.Sprintf("Banned word %q added.", wordToAdd))

	case "del", "remove":
		if len(args) < 2 {
			return ctx.Reply("Please specify the word to remove.")
		}
		wordToDel := strings.ToLower(args[1])
		found := false
		var newWords []string
		for _, w := range words {
			if w == wordToDel {
				found = true
			} else {
				newWords = append(newWords, w)
			}
		}
		if !found {
			return ctx.Reply(fmt.Sprintf("Word %q was not banned.", wordToDel))
		}
		_ = s.PutSetting(ctx.Ctx, settingKey, strings.Join(newWords, " "))
		return sendAntiWordMenu(ctx, s, fmt.Sprintf("Banned word %q removed.", wordToDel))

	case "list":
		if len(words) == 0 {
			return ctx.Reply("No banned words configured in this group.")
		}
		return ctx.Reply(fmt.Sprintf("Banned Words list for %s:\n- %s", groupName, strings.Join(words, "\n- ")))

	default:
		return sendAntiWordMenu(ctx, s, "")
	}
}

func sendAntiWordMenu(ctx *Context, s *sqlstore.SQLStore, note string) error {
	chatKey := ctx.Chat.String()
	groupName := chatKey
	info, err := ctx.Client.GetGroupInfo(ctx.Ctx, ctx.Chat)
	if err == nil && info != nil && info.Name != "" {
		groupName = info.Name
	}

	status, _ := s.GetSetting(ctx.Ctx, "antiword_status:"+chatKey)
	rawWord, _ := s.GetSetting(ctx.Ctx, "antiword:"+chatKey)
	words := strings.Fields(rawWord)
	if status == "" {
		if len(words) > 0 {
			status = "on"
		} else {
			status = "off"
		}
	}

	action, _ := s.GetSetting(ctx.Ctx, "antiword_action:"+chatKey)
	if action == "" {
		action = "delete"
	}
	actionDisplay := strings.ToUpper(action)
	if action == "warn" {
		maxWarn, _ := s.GetSetting(ctx.Ctx, "antiword_maxwarn:"+chatKey)
		if maxWarn == "" {
			maxWarn = "3"
		}
		actionDisplay = fmt.Sprintf("WARN (Max: %s)", maxWarn)
	}

	p := ctx.GetPrefix()
	var sb strings.Builder
	fmt.Fprintf(&sb, "╭━━━〔 ANTIWORD CONFIGURATION 〕━━━\n│ Group  : %s\n│ Status : %s\n│ Action : %s\n│ Banned : %d word(s)\n╰━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n", groupName, strings.ToUpper(status), actionDisplay, len(words))

	if note != "" {
		sb.WriteString(note)
		sb.WriteString("\n\n")
	}

	if len(words) > 0 {
		fmt.Fprintf(&sb, "Banned Words: %s\n\n", strings.Join(words, ", "))
	}

	sb.WriteString("Options:\n")
	fmt.Fprintf(&sb, "• `%santiword add <word>` - Add banned word\n", p)
	fmt.Fprintf(&sb, "• `%santiword del <word>` - Remove banned word\n", p)
	fmt.Fprintf(&sb, "• `%santiword action <delete|kick|warn>` - Set action mode\n", p)
	fmt.Fprintf(&sb, "• `%santiword setwarn 3` - Set warning limit\n", p)

	var toggleBtn struct{ ID, Text string }
	if status == "on" {
		toggleBtn = struct{ ID, Text string }{ID: p + "antiword off", Text: "Deactivate"}
	} else {
		toggleBtn = struct{ ID, Text string }{ID: p + "antiword on", Text: "Activate"}
	}

	buttons := []struct{ ID, Text string }{
		toggleBtn,
		{ID: p + "antiword action", Text: "Action Mode"},
		{ID: p + "antiword list", Text: "List Words"},
	}

	return sendInteractiveButtons(ctx, strings.TrimSpace(sb.String()), fmt.Sprintf("%s AntiWord Moderation", ctx.GetBotName()), buttons)
}

func handleGStats(ctx *Context) error {
	s, ok := ctx.Client.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return ctx.Reply("Settings store unavailable.")
	}
	db := s.GetDB()
	if db == nil {
		return ctx.Reply("Database unavailable.")
	}

	chatStr := ctx.Chat.String()

	var totalMsgs int
	err := db.QueryRow(ctx.Ctx, `SELECT COUNT(*) FROM message_secrets WHERE chat_jid=$1`, chatStr).Scan(&totalMsgs)
	if err != nil {
		return err
	}

	if totalMsgs == 0 {
		return ctx.Reply("No message activity found in database for this group.")
	}

	var activeUsers int
	err = db.QueryRow(ctx.Ctx, `SELECT COUNT(DISTINCT sender_jid) FROM message_secrets WHERE chat_jid=$1`, chatStr).Scan(&activeUsers)
	if err != nil {
		activeUsers = 0
	}

	rows, err := db.Query(ctx.Ctx, `
		SELECT sender_jid, COUNT(*) as total 
		FROM message_secrets 
		WHERE chat_jid=$1 
		GROUP BY sender_jid 
		ORDER BY total DESC 
		LIMIT 10
	`, chatStr)
	if err != nil {
		return err
	}
	defer rows.Close()

	var mentions []types.JID
	var sb strings.Builder
	sb.WriteString(" *Group Activity Statistics (from message secrets)*\n\n")
	sb.WriteString(fmt.Sprintf("• Total messages tracked: %d\n", totalMsgs))
	sb.WriteString(fmt.Sprintf("• Unique active senders: %d\n\n", activeUsers))
	sb.WriteString(" *Top Active Participants:*\n")

	rank := 1
	for rows.Next() {
		var userStr string
		var count int
		if err := rows.Scan(&userStr, &count); err == nil {
			uj, err := types.ParseJID(userStr)
			if err == nil {
				uj = uj.ToNonAD()
				resolvedJID, username := ctx.ResolveMention(uj)
				fmt.Fprintf(&sb, "%d. @%s (%d msgs)\n", rank, username, count)
				mentions = append(mentions, resolvedJID)
				rank++
			}
		}
	}

	return ctx.ReplyWithMentions(sb.String(), mentions)
}

func handlePoll(ctx *Context) error {
	raw := strings.TrimSpace(ctx.RawArgs)
	if raw == "" {
		p := ctx.GetPrefix()
		return ctx.Reply(fmt.Sprintf("Usage: %spoll Question | Option 1 | Option 2 | ...", p))
	}

	selectableCount := -1
	if strings.HasPrefix(raw, "--single ") || strings.HasPrefix(raw, "-s ") || strings.HasPrefix(raw, "single ") {
		selectableCount = 1
		raw = strings.TrimSpace(raw[strings.Index(raw, " "):])
	} else if strings.HasPrefix(raw, "--multi ") || strings.HasPrefix(raw, "-m ") || strings.HasPrefix(raw, "multi ") || strings.HasPrefix(raw, "multiple ") {
		selectableCount = 0
		raw = strings.TrimSpace(raw[strings.Index(raw, " "):])
	}

	parts := strings.Split(raw, "|")
	if len(parts) < 3 {
		p := ctx.GetPrefix()
		return ctx.Reply(fmt.Sprintf("Usage: %spoll Question | Option 1 | Option 2 | ...", p))
	}

	question := strings.TrimSpace(parts[0])
	var options []string
	for _, opt := range parts[1:] {
		trimmed := strings.TrimSpace(opt)
		if trimmed != "" {
			options = append(options, trimmed)
		}
	}
	if len(options) < 2 {
		return ctx.Reply("Please provide at least 2 options.")
	}

	if selectableCount >= 0 {
		pollMsg := ctx.Client.BuildPollCreation(question, options, selectableCount)
		_, err := ctx.Client.SendMessage(ctx.Ctx, ctx.Chat, pollMsg)
		return err
	}

	var sb strings.Builder
	sb.WriteString("Poll Creation\n\nQuestion: ")
	sb.WriteString(question)
	sb.WriteString("\nOptions:\n")
	for i, opt := range options {
		fmt.Fprintf(&sb, "%d. %s\n", i+1, opt)
	}
	sb.WriteString("\nSelect poll type below to create poll.")

	p := ctx.GetPrefix()
	pollArgs := question + " | " + strings.Join(options, " | ")
	msg := &waE2E.Message{
		DocumentWithCaptionMessage: &waE2E.FutureProofMessage{
			Message: &waE2E.Message{
				ButtonsMessage: &waE2E.ButtonsMessage{
					ContentText: new(sb.String()),
					FooterText:  new(fmt.Sprintf("%s Interactive Poll", ctx.GetBotName())),
					HeaderType:  waE2E.ButtonsMessage_EMPTY.Enum(),
					Buttons: []*waE2E.ButtonsMessage_Button{
						{
							ButtonID: new(p + "poll --single " + pollArgs),
							ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{
								DisplayText: new("SINGLE CHOICE"),
							},
							Type: waE2E.ButtonsMessage_Button_RESPONSE.Enum(),
						},
						{
							ButtonID: new(p + "poll --multi " + pollArgs),
							ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{
								DisplayText: new("MULTIPLE CHOICE"),
							},
							Type: waE2E.ButtonsMessage_Button_RESPONSE.Enum(),
						},
					},
				},
			},
		},
	}

	bizNode := waBinary.Node{
		Tag:   "biz",
		Attrs: waBinary.Attrs{},
		Content: []waBinary.Node{
			{
				Tag: "interactive",
				Attrs: waBinary.Attrs{
					"type": "native_flow",
					"v":    "1",
				},
				Content: []waBinary.Node{
					{
						Tag: "native_flow",
						Attrs: waBinary.Attrs{
							"v":    "9",
							"name": "mixed",
						},
					},
				},
			},
		},
	}

	extra := whatsmeow.SendRequestExtra{
		AdditionalNodes: &[]waBinary.Node{bizNode},
	}

	_, err := ctx.Client.SendMessage(ctx.Ctx, ctx.Chat, msg, extra)
	return err
}

func handleInvite(ctx *Context) error {
	info, err := ctx.Client.GetGroupInfo(ctx.Ctx, ctx.Chat)
	if err != nil {
		return ctx.Reply(fmt.Sprintf("Failed to get group info: %v", err))
	}
	if !ctx.IsSenderAdmin(info) {
		return ctx.Reply("Only group admins can retrieve the invite link.")
	}

	link, err := ctx.Client.GetGroupInviteLink(ctx.Ctx, ctx.Chat, false)
	if err != nil {
		return ctx.Reply(fmt.Sprintf("Failed to get invite link: %v", err))
	}
	return ctx.Reply(link)
}

var (
	presenceMu  sync.RWMutex
	presenceMap = make(map[string]PresenceInfo)
)

type PresenceInfo struct {
	LastSeen time.Time
	IsOnline bool
}

func TrackPresence(jid types.JID, isOnline bool) {
	if jid.IsEmpty() {
		return
	}
	key := jid.ToNonAD().String()
	presenceMu.Lock()
	presenceMap[key] = PresenceInfo{
		LastSeen: time.Now(),
		IsOnline: isOnline,
	}
	presenceMu.Unlock()
}

func IsUserOnline(jid types.JID, client *whatsmeow.Client) bool {
	if jid.IsEmpty() {
		return false
	}
	targetKey := jid.ToNonAD().String()

	presenceMu.RLock()
	info, exists := presenceMap[targetKey]
	presenceMu.RUnlock()

	if exists && (info.IsOnline || time.Since(info.LastSeen) < 15*time.Minute) {
		slog.Debug("IsUserOnline check: direct match online", "jid", targetKey, "lastSeen", info.LastSeen)
		return true
	}

	if client != nil && client.Store != nil && client.Store.LIDs != nil {
		ctx := context.Background()
		if jid.Server == types.HiddenUserServer {
			pn, err := client.Store.LIDs.GetPNForLID(ctx, jid)
			if err == nil && !pn.IsEmpty() {
				pnKey := pn.ToNonAD().String()
				presenceMu.RLock()
				pnInfo, pnExists := presenceMap[pnKey]
				presenceMu.RUnlock()
				if pnExists && (pnInfo.IsOnline || time.Since(pnInfo.LastSeen) < 15*time.Minute) {
					slog.Debug("IsUserOnline check: PN match online for LID", "lid", targetKey, "pn", pnKey)
					return true
				}
			}
		} else {
			lid, err := client.Store.LIDs.GetLIDForPN(ctx, jid)
			if err == nil && !lid.IsEmpty() {
				lidKey := lid.ToNonAD().String()
				presenceMu.RLock()
				lidInfo, lidExists := presenceMap[lidKey]
				presenceMu.RUnlock()
				if lidExists && (lidInfo.IsOnline || time.Since(lidInfo.LastSeen) < 15*time.Minute) {
					slog.Debug("IsUserOnline check: LID match online for PN", "pn", targetKey, "lid", lidKey)
					return true
				}
			}
		}
	}

	slog.Debug("IsUserOnline check: offline or unknown", "jid", targetKey)
	return false
}

func handleListOnline(ctx *Context) error {
	if ctx.Chat.Server != "g.us" {
		slog.Debug("handleListOnline: not a group chat", "chat", ctx.Chat.String())
		return ctx.Reply("This command can only be used in a group.")
	}

	slog.Debug("handleListOnline executing", "group", ctx.Chat.String())
	info, err := ctx.Client.GetGroupInfo(ctx.Ctx, ctx.Chat)
	if err != nil {
		slog.Error("handleListOnline: failed to get group info", "group", ctx.Chat.String(), "err", err)
		return ctx.Reply(fmt.Sprintf("Failed to get group info: %v", err))
	}

	total := len(info.Participants)
	slog.Debug("handleListOnline retrieved group info", "group", ctx.Chat.String(), "participant_count", total)

	if total == 0 {
		return ctx.Reply("No participants found in this group.")
	}

	// 1. Send status message to prompt WhatsApp servers to trigger group-wide delivery receipts
	_ = ctx.Reply("Fetching online participants...")

	// 2. Query SQLite database for stored receipt/presence activity for this group
	if s, ok := ctx.Client.Store.Identities.(*sqlstore.SQLStore); ok {
		dbActive, err := s.GetActiveGroupParticipants(ctx.Ctx, ctx.Chat, 24*time.Hour)
		if err == nil && len(dbActive) > 0 {
			slog.Debug("handleListOnline: loaded stored active participants from SQLite", "count", len(dbActive))
			for _, userJID := range dbActive {
				TrackPresence(userJID, true)
			}
		}
	}

	// Build set of expected participant JID keys (LID & PN formats)
	expectedJIDs := make(map[string]types.JID)
	var mu sync.Mutex
	receivedCount := 0
	doneChan := make(chan struct{})

	for _, p := range info.Participants {
		nonAD := p.JID.ToNonAD()
		expectedJIDs[nonAD.String()] = nonAD
		if ctx.Client.Store != nil && ctx.Client.Store.LIDs != nil {
			if nonAD.Server == types.HiddenUserServer {
				if pn, err := ctx.Client.Store.LIDs.GetPNForLID(ctx.Ctx, nonAD); err == nil && !pn.IsEmpty() {
					expectedJIDs[pn.ToNonAD().String()] = nonAD
				}
			} else {
				if lid, err := ctx.Client.Store.LIDs.GetLIDForPN(ctx.Ctx, nonAD); err == nil && !lid.IsEmpty() {
					expectedJIDs[lid.ToNonAD().String()] = nonAD
				}
			}
		}
	}

	// Register temporary event listener for WhatsApp presence and receipt response stanzas
	handlerID := ctx.Client.AddEventHandler(func(evt any) {
		switch pEvt := evt.(type) {
		case *events.Presence:
			fromKey := pEvt.From.ToNonAD().String()
			mu.Lock()
			if targetJID, isExpected := expectedJIDs[fromKey]; isExpected {
				slog.Debug("handleListOnline: received presence stanza from WhatsApp", "from", fromKey, "unavailable", pEvt.Unavailable)
				TrackPresence(targetJID, !pEvt.Unavailable)
				delete(expectedJIDs, fromKey)
				receivedCount++
				if len(expectedJIDs) == 0 {
					select {
					case <-doneChan:
					default:
						close(doneChan)
					}
				}
			}
			mu.Unlock()

		case *events.Receipt:
			senderKey := pEvt.Sender.ToNonAD().String()
			if !pEvt.Sender.IsEmpty() {
				mu.Lock()
				if targetJID, isExpected := expectedJIDs[senderKey]; isExpected {
					slog.Debug("handleListOnline: received delivery receipt from WhatsApp", "sender", senderKey)
					TrackPresence(targetJID, true)
					delete(expectedJIDs, senderKey)
					receivedCount++
					if len(expectedJIDs) == 0 {
						select {
						case <-doneChan:
						default:
							close(doneChan)
						}
					}
				}
				mu.Unlock()
			}
		}
	})
	defer ctx.Client.RemoveEventHandler(handlerID)

	// Dispatch SubscribePresence to WhatsApp for all group participants
	for _, p := range info.Participants {
		_ = ctx.Client.SubscribePresence(ctx.Ctx, p.JID)
	}

	// Check if presenceMap already has cached online presence for group members
	cachedOnlineCount := 0
	for _, p := range info.Participants {
		if IsUserOnline(p.JID, ctx.Client) {
			cachedOnlineCount++
		}
	}

	// If cached online records are small, wait up to 2s for live presence/receipt stanzas
	if cachedOnlineCount < 2 {
		select {
		case <-doneChan:
			slog.Debug("handleListOnline: presence/receipt stanzas collected", "count", receivedCount)
		case <-time.After(2000 * time.Millisecond):
			slog.Debug("handleListOnline: presence wait window ended", "received", receivedCount, "total", total)
		}
	}

	// Collect online participants
	var onlineJIDs []types.JID
	var displayNames []string

	for _, p := range info.Participants {
		if IsUserOnline(p.JID, ctx.Client) {
			resolvedJID, username := ctx.ResolveMention(p.JID)
			onlineJIDs = append(onlineJIDs, resolvedJID)
			displayNames = append(displayNames, "@"+username)
			slog.Debug("handleListOnline: participant online", "participant", p.JID.String(), "username", username)
		} else {
			slog.Debug("handleListOnline: participant offline", "participant", p.JID.String())
		}
	}

	slog.Debug("handleListOnline complete", "group", ctx.Chat.String(), "total_participants", total, "online_count", len(onlineJIDs))

	if len(onlineJIDs) == 0 {
		return ctx.Reply("No online participants detected in this group.")
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Online Participants (%d):\n\n", len(onlineJIDs)))
	for _, name := range displayNames {
		fmt.Fprintf(&sb, "- %s\n", name)
	}

	return ctx.ReplyWithMentions(sb.String(), onlineJIDs)
}

func handleKickAll(ctx *Context) error {
	if ctx.Chat.Server != "g.us" {
		return ctx.Reply("This command can only be used in a group.")
	}

	info, err := ctx.Client.GetGroupInfo(ctx.Ctx, ctx.Chat)
	if err != nil {
		return ctx.Reply(fmt.Sprintf("Failed to get group info: %v", err))
	}

	if !ctx.IsSenderAdmin(info) && !ctx.IsSudo() {
		return ctx.Reply("Only group admins or bot owners can use kickall.")
	}

	botJID := ctx.Client.Store.ID.ToNonAD()
	botLID := ctx.Client.Store.LID.ToNonAD()

	botIsAdmin := false
	for _, p := range info.Participants {
		if (p.JID.User == botJID.User || (!botLID.IsEmpty() && p.JID.User == botLID.User)) && p.IsAdmin {
			botIsAdmin = true
			break
		}
	}

	if !botIsAdmin {
		return ctx.Reply("I need admin privileges to kick participants.")
	}

	var toKick []types.JID
	for _, p := range info.Participants {
		if p.JID.User == botJID.User || (!botLID.IsEmpty() && p.JID.User == botLID.User) {
			continue
		}
		if p.JID.User == ctx.Sender.ToNonAD().User {
			continue
		}
		if isJIDSudo(ctx, p.JID) {
			continue
		}
		toKick = append(toKick, p.JID)
	}

	if len(toKick) == 0 {
		return ctx.Reply("No participants to kick.")
	}

	_ = ctx.Reply(fmt.Sprintf("Kicking %d participants...", len(toKick)))
	_, err = ctx.Client.UpdateGroupParticipants(ctx.Ctx, ctx.Chat, toKick, whatsmeow.ParticipantChangeRemove)
	if err != nil {
		slog.Error("Kickall failed", "err", err)
		return ctx.Reply(fmt.Sprintf("Failed to kick participants: %v", err))
	}

	return ctx.Reply(fmt.Sprintf("Kickall complete! Removed %d participants.", len(toKick)))
}

func handleCommunity(ctx *Context) error {
	groups, err := ctx.Client.GetJoinedGroups(ctx.Ctx)
	if err != nil || len(groups) == 0 {
		return ctx.Reply("Failed to fetch joined groups or no groups joined.")
	}

	var sb strings.Builder
	sb.WriteString("╭━━━〔 COMMUNITY GROUPS 〕━━━\n│\n")

	count := 0
	for i, g := range groups {
		groupName := g.Name
		if groupName == "" && g.GroupName.Name != "" {
			groupName = g.GroupName.Name
		}
		if groupName == "" {
			groupName = fmt.Sprintf("Group %d", i+1)
		}

		link := "Invite link unavailable"
		if code, errL := ctx.Client.GetGroupInviteLink(ctx.Ctx, g.JID, false); errL == nil && code != "" {
			link = "https://chat.whatsapp.com/" + code
		}

		count++
		fmt.Fprintf(&sb, "│ %d. %s\n│    Link: %s\n│\n", count, groupName, link)
	}

	sb.WriteString("╰━━━━━━━━━━━━━━━━━━━━━━━")
	return ctx.Reply(strings.TrimSpace(sb.String()))
}

func handleLeave(ctx *Context) error {
	if ctx.Chat.Server != "g.us" {
		return ctx.Reply("This command can only be used in a group.")
	}

	p := ctx.GetPrefix()
	senderUser := ctx.Sender.ToNonAD().User

	arg0 := ""
	if len(ctx.Args) > 0 {
		arg0 = strings.ToLower(ctx.Args[0])
	}

	if strings.HasPrefix(arg0, "confirm") {
		parts := strings.Split(arg0, "_")
		if len(parts) >= 2 {
			callerUser := parts[1]
			if senderUser != callerUser && !ctx.IsSudo() {
				callerMention, _ := ctx.ResolveMention(types.NewJID(callerUser, "s.whatsapp.net"))
				return ctx.ReplyWithMentions(fmt.Sprintf("Only the command caller (%s) can confirm leaving this group.", "@"+callerMention.User), []types.JID{callerMention})
			}
		}

		_ = ctx.Reply("Leaving group... Goodbye!")
		err := ctx.Client.LeaveGroup(ctx.Ctx, ctx.Chat)
		if err != nil {
			slog.Error("Failed to leave group", "err", err)
			return ctx.Reply(fmt.Sprintf("Failed to leave group: %v", err))
		}
		return nil
	}

	if strings.HasPrefix(arg0, "cancel") {
		parts := strings.Split(arg0, "_")
		if len(parts) >= 2 {
			callerUser := parts[1]
			if senderUser != callerUser && !ctx.IsSudo() {
				callerMention, _ := ctx.ResolveMention(types.NewJID(callerUser, "s.whatsapp.net"))
				return ctx.ReplyWithMentions(fmt.Sprintf("Only the command caller (%s) can cancel leaving.", "@"+callerMention.User), []types.JID{callerMention})
			}
		}
		return ctx.Reply("Leave group cancelled.")
	}

	confirmBtnID := fmt.Sprintf("%sleave confirm_%s", p, senderUser)
	cancelBtnID := fmt.Sprintf("%sleave cancel_%s", p, senderUser)

	bodyText := "⚠️ ARE YOU SURE YOU WANT ME TO LEAVE THIS GROUP?\n\nClick 'Confirm Leave' below to confirm or 'Cancel' to keep me in the group."
	buttons := []struct{ ID, Text string }{
		{ID: confirmBtnID, Text: "Confirm Leave"},
		{ID: cancelBtnID, Text: "Cancel"},
	}

	return sendInteractiveButtons(ctx, bodyText, fmt.Sprintf("Powered by %s", ctx.GetBotName()), buttons)
}

var groupInviteLinkRegex = regexp.MustCompile(`(?i)(?:https?://)?chat\.whatsapp\.com/([A-Za-z0-9_-]+)`)

func handleJoin(ctx *Context) error {
	var inviteMsg *waE2E.GroupInviteMessage
	var isQuoted bool

	if quoted := ctx.GetQuotedMessage(); quoted != nil && quoted.GetGroupInviteMessage() != nil {
		inviteMsg = quoted.GetGroupInviteMessage()
		isQuoted = true
	} else if ctx.Evt != nil && ctx.Evt.Message != nil && ctx.Evt.Message.GetGroupInviteMessage() != nil {
		inviteMsg = ctx.Evt.Message.GetGroupInviteMessage()
	}

	if inviteMsg != nil {
		return handleJoinV4(ctx, inviteMsg, isQuoted)
	}

	code := extractGroupInviteCode(ctx)
	if code == "" {
		return ErrUsage("join <group_url>")
	}

	jid, err := ctx.Client.JoinGroupWithLink(ctx.GetSendContext(), code)
	if err != nil {
		return ctx.Reply(fmt.Sprintf("Failed to join group: %v", err))
	}

	groupName := ""
	if info, errInfo := ctx.Client.GetGroupInfo(ctx.GetSendContext(), jid); errInfo == nil && info != nil && info.Name != "" {
		groupName = info.Name
	}

	if groupName != "" {
		return ctx.Reply(fmt.Sprintf("Successfully joined group: *%s*", groupName))
	}
	return ctx.Reply("Successfully joined the group!")
}

func handleJoinV4(ctx *Context, inviteMsg *waE2E.GroupInviteMessage, isQuoted bool) error {
	groupJIDStr := inviteMsg.GetGroupJID()
	groupJID, err := types.ParseJID(groupJIDStr)
	if err != nil || groupJID.IsEmpty() {
		return ctx.Reply("Invalid group JID in invite message.")
	}

	code := inviteMsg.GetInviteCode()
	if code == "" {
		return ctx.Reply("Invalid invite code in invite message.")
	}

	expiration := inviteMsg.GetInviteExpiration()

	var inviterJID types.JID
	if isQuoted {
		if sender, ok := ctx.GetQuotedSender(); ok && !sender.IsEmpty() {
			inviterJID = sender
		}
	}
	if inviterJID.IsEmpty() {
		if ci := inviteMsg.GetContextInfo(); ci != nil && ci.GetParticipant() != "" {
			if parsed, errP := types.ParseJID(ci.GetParticipant()); errP == nil && !parsed.IsEmpty() {
				inviterJID = parsed
			}
		}
	}
	if inviterJID.IsEmpty() {
		inviterJID = ctx.Sender
	}

	err = ctx.Client.JoinGroupWithInvite(ctx.GetSendContext(), groupJID, inviterJID, code, expiration)
	if err != nil {
		return ctx.Reply(fmt.Sprintf("Failed to join group via invite: %v", err))
	}

	groupName := inviteMsg.GetGroupName()
	if groupName == "" {
		if info, errInfo := ctx.Client.GetGroupInfo(ctx.GetSendContext(), groupJID); errInfo == nil && info != nil && info.Name != "" {
			groupName = info.Name
		}
	}

	if groupName != "" {
		return ctx.Reply(fmt.Sprintf("Successfully joined group: *%s*", groupName))
	}
	return ctx.Reply("Successfully joined the group!")
}

func extractGroupInviteCode(ctx *Context) string {
	if ctx == nil {
		return ""
	}
	if ctx.RawArgs != "" {
		if match := groupInviteLinkRegex.FindStringSubmatch(ctx.RawArgs); len(match) > 1 {
			return match[1]
		}
		trimmed := strings.TrimSpace(ctx.RawArgs)
		if !strings.ContainsAny(trimmed, " \t\n/\\") && len(trimmed) >= 10 && len(trimmed) <= 32 {
			return trimmed
		}
	}

	if quoted := ctx.GetQuotedMessage(); quoted != nil {
		text := extractMessageText(quoted)
		if match := groupInviteLinkRegex.FindStringSubmatch(text); len(match) > 1 {
			return match[1]
		}
		trimmed := strings.TrimSpace(text)
		if !strings.ContainsAny(trimmed, " \t\n/\\") && len(trimmed) >= 10 && len(trimmed) <= 32 {
			return trimmed
		}
	}

	if ctx.Evt != nil && ctx.Evt.Message != nil {
		text := extractMessageText(ctx.Evt.Message)
		if match := groupInviteLinkRegex.FindStringSubmatch(text); len(match) > 1 {
			return match[1]
		}
	}

	return ""
}

func extractMessageText(msg *waE2E.Message) string {
	if msg == nil {
		return ""
	}
	if text := msg.GetConversation(); text != "" {
		return text
	}
	if ext := msg.GetExtendedTextMessage(); ext != nil && ext.GetText() != "" {
		return ext.GetText()
	}
	if img := msg.GetImageMessage(); img != nil && img.GetCaption() != "" {
		return img.GetCaption()
	}
	if vid := msg.GetVideoMessage(); vid != nil && vid.GetCaption() != "" {
		return vid.GetCaption()
	}
	if doc := msg.GetDocumentMessage(); doc != nil && doc.GetCaption() != "" {
		return doc.GetCaption()
	}
	if inv := msg.GetGroupInviteMessage(); inv != nil && inv.GetCaption() != "" {
		return inv.GetCaption()
	}
	return ""
}
