// Warn command – issue warnings to users, set warning thresholds, and automatically block & kick offenders.
package plugins

import (
	"fmt"
	"strconv"
	"strings"

	"whatsrook/send"
	"whatsrook/wa-core/store/sqlstore"

	"whatsrook/wa-core"
	"whatsrook/wa-core/types"
	"whatsrook/wa-core/types/events"
)

func init() {
	Register(&Command{
		Name:        "warn",
		Aliases:     []string{"warning"},
		Description: "Issue a warning to a participant. Blocks and kicks when max warning threshold is reached.",
		Category:    "group",
		IsPublic:    false,
		Handler:     handleWarn,
	})

	Register(&Command{
		Name:        "unwarn",
		Aliases:     []string{"delwarn", "resetwarn", "rmwarn"},
		Description: "Remove warnings from a participant",
		Category:    "group",
		IsPublic:    false,
		Handler:     handleUnwarn,
	})

	Register(&Command{
		Name:        "warns",
		Aliases:     []string{"getwarn", "listwarns"},
		Description: "Check current warning count for a participant or group",
		Category:    "group",
		IsPublic:    true,
		Handler:     handleWarns,
	})

	Register(&Command{
		Name:        "setwarn",
		Aliases:     []string{"warnlimit", "setwarnlimit"},
		Description: "Set max warning threshold before taking automated block/kick action",
		Category:    "group",
		IsPublic:    false,
		Handler:     handleSetWarn,
	})
}

func handleWarn(ctx *Context) error {
	s, ok := ctx.Client.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return ctx.Reply("Database store not available.")
	}

	args := strings.Fields(ctx.RawArgs)
	ci := ctx.GetContextInfo()
	hasMention := ci != nil && len(ci.GetMentionedJID()) > 0

	if len(args) == 0 && ctx.GetQuotedMessage() == nil && !hasMention {
		return sendWarnMenu(ctx, s)
	}

	if len(args) > 0 {
		sub := strings.ToLower(args[0])
		if sub == "customize" || sub == "custom" || sub == "help" {
			return sendWarnCustomizeGuide(ctx)
		}
		if sub == "limit" || sub == "max" || sub == "set" {
			if len(args) > 1 {
				ctx.RawArgs = strings.Join(args[1:], " ")
			} else {
				ctx.RawArgs = ""
			}
			return handleSetWarn(ctx)
		}
	}

	targetJID := extractWarnTarget(ctx, args)
	if targetJID.IsEmpty() {
		p := ctx.GetPrefix()
		return ctx.Reply(fmt.Sprintf("Usage:\n- %swarn @user [reason]\n- Reply to a message with %swarn\n- %swarn 1234567890", p, p, p))
	}

	// 1. Immunity check: Bot owner & sudoers cannot be warned
	if isJIDOwnerOrSudo(ctx, targetJID) {
		return ctx.Reply("You cannot issue warnings to the bot owner or sudoers.")
	}

	isGroup := ctx.Chat.Server == "g.us"
	var groupInfo *types.GroupInfo
	if isGroup {
		var err error
		groupInfo, err = ctx.Client.GetGroupInfo(ctx.Ctx, ctx.Chat)
		if err != nil {
			return ctx.Reply(fmt.Sprintf("Failed to fetch group info: %v", err))
		}

		// Non-owners cannot warn group admins
		if isParticipantAdmin(groupInfo, targetJID) && !ctx.IsSudo() {
			return ctx.Reply("You cannot issue warnings to group admins.")
		}
	}

	chatKey := ctx.Chat.String()
	userKey := targetJID.ToNonAD().User
	warnKey := fmt.Sprintf("warn_count:%s:%s", chatKey, userKey)
	limitKey := fmt.Sprintf("warn_limit:%s", chatKey)

	rawCount, _ := s.GetSetting(ctx.Ctx, warnKey)
	currentWarns, _ := strconv.Atoi(rawCount)
	currentWarns++
	_ = s.PutSetting(ctx.Ctx, warnKey, strconv.Itoa(currentWarns))

	rawLimit, _ := s.GetSetting(ctx.Ctx, limitKey)
	maxLimit, _ := strconv.Atoi(rawLimit)
	if maxLimit <= 0 {
		maxLimit = 3
	}

	resolvedJID, username := ctx.ResolveMention(targetJID)
	reason := ""
	if len(args) > 1 && !strings.HasPrefix(args[0], "@") {
		reason = strings.Join(args[1:], " ")
	} else if len(args) > 1 && strings.HasPrefix(args[0], "@") {
		reason = strings.Join(args[1:], " ")
	}

	if currentWarns < maxLimit {
		msg := fmt.Sprintf("⚠️ Warning issued to @%s (%d/%d).", username, currentWarns, maxLimit)
		if reason != "" {
			msg += "\nReason: " + reason
		}
		return ctx.ReplyWithMentions(msg, []types.JID{resolvedJID})
	}

	// Warning limit reached (currentWarns >= maxLimit)
	if isGroup {
		// Admin kick restriction check
		targetIsAdmin := isParticipantAdmin(groupInfo, targetJID)
		botIsOwner := isBotGroupOwner(ctx, groupInfo)

		if targetIsAdmin && !botIsOwner {
			return ctx.ReplyWithMentions(fmt.Sprintf("⚠️ @%s reached the maximum warning limit (%d/%d), but cannot be kicked/blocked because they are a group admin and I am not the group owner.", username, currentWarns, maxLimit), []types.JID{resolvedJID})
		}

		if !isBotAdmin(ctx, groupInfo) {
			return ctx.ReplyWithMentions(fmt.Sprintf("⚠️ @%s reached the maximum warning limit (%d/%d), but I require admin privileges to block and kick them.", username, currentWarns, maxLimit), []types.JID{resolvedJID})
		}

		// Execute Block & Kick
		_, _ = ctx.Client.UpdateBlocklist(ctx.Ctx, targetJID, events.BlocklistChangeActionBlock)
		_, err := ctx.Client.UpdateGroupParticipants(ctx.Ctx, ctx.Chat, []types.JID{targetJID}, whatsmeow.ParticipantChangeRemove)
		if err != nil {
			return ctx.Reply(fmt.Sprintf("Failed to kick @%s from group: %v", username, err))
		}

		_ = s.PutSetting(ctx.Ctx, warnKey, "0")
		return ctx.ReplyWithMentions(fmt.Sprintf("🚨 @%s reached maximum warnings (%d/%d) and has been blocked and kicked from the group.", username, currentWarns, maxLimit), []types.JID{resolvedJID})
	}

	// Private Chat (DM)
	_, _ = ctx.Client.UpdateBlocklist(ctx.Ctx, targetJID, events.BlocklistChangeActionBlock)
	_ = s.PutSetting(ctx.Ctx, warnKey, "0")
	return ctx.ReplyWithMentions(fmt.Sprintf("🚨 User @%s reached maximum warnings (%d/%d) and has been blocked.", username, currentWarns, maxLimit), []types.JID{resolvedJID})
}

func handleUnwarn(ctx *Context) error {
	s, ok := ctx.Client.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return ctx.Reply("Database store not available.")
	}

	args := strings.Fields(ctx.RawArgs)
	targetJID := extractWarnTarget(ctx, args)
	if targetJID.IsEmpty() {
		return ctx.Reply("Please mention a participant or quote their message to remove warnings.")
	}

	chatKey := ctx.Chat.String()
	userKey := targetJID.ToNonAD().User
	warnKey := fmt.Sprintf("warn_count:%s:%s", chatKey, userKey)

	rawCount, _ := s.GetSetting(ctx.Ctx, warnKey)
	currentWarns, _ := strconv.Atoi(rawCount)
	if currentWarns <= 0 {
		resolvedJID, username := ctx.ResolveMention(targetJID)
		return ctx.ReplyWithMentions(fmt.Sprintf("@%s has 0 active warnings.", username), []types.JID{resolvedJID})
	}

	currentWarns--
	_ = s.PutSetting(ctx.Ctx, warnKey, strconv.Itoa(currentWarns))
	resolvedJID, username := ctx.ResolveMention(targetJID)
	return ctx.ReplyWithMentions(fmt.Sprintf(" Removed 1 warning from @%s. Remaining warnings: %d.", username, currentWarns), []types.JID{resolvedJID})
}

func handleWarns(ctx *Context) error {
	s, ok := ctx.Client.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return ctx.Reply("Database store not available.")
	}

	args := strings.Fields(ctx.RawArgs)
	targetJID := extractWarnTarget(ctx, args)
	chatKey := ctx.Chat.String()
	limitKey := fmt.Sprintf("warn_limit:%s", chatKey)
	rawLimit, _ := s.GetSetting(ctx.Ctx, limitKey)
	maxLimit, _ := strconv.Atoi(rawLimit)
	if maxLimit <= 0 {
		maxLimit = 3
	}

	if !targetJID.IsEmpty() {
		userKey := targetJID.ToNonAD().User
		warnKey := fmt.Sprintf("warn_count:%s:%s", chatKey, userKey)
		rawCount, _ := s.GetSetting(ctx.Ctx, warnKey)
		currentWarns, _ := strconv.Atoi(rawCount)

		resolvedJID, username := ctx.ResolveMention(targetJID)
		return ctx.ReplyWithMentions(fmt.Sprintf("Participant @%s has %d/%d warnings.", username, currentWarns, maxLimit), []types.JID{resolvedJID})
	}

	p := ctx.GetPrefix()
	return ctx.Reply(fmt.Sprintf("Max Warning Threshold for this chat: %d warnings.\nUsage: %swarns @user to check specific participant warnings.", maxLimit, p))
}

func handleSetWarn(ctx *Context) error {
	s, ok := ctx.Client.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return ctx.Reply("Database store not available.")
	}

	args := strings.Fields(ctx.RawArgs)
	if len(args) == 0 {
		p := ctx.GetPrefix()
		return ctx.Reply(fmt.Sprintf("Usage: %ssetwarn <count> (e.g. %ssetwarn 3)", p, p))
	}

	num, err := strconv.Atoi(args[0])
	if err != nil || num < 1 || num > 20 {
		return ctx.Reply("Please specify a valid warning limit count between 1 and 20.")
	}

	chatKey := ctx.Chat.String()
	limitKey := fmt.Sprintf("warn_limit:%s", chatKey)
	if err := s.PutSetting(ctx.Ctx, limitKey, strconv.Itoa(num)); err != nil {
		return ctx.Reply("Failed to update warning threshold.")
	}

	return ctx.Reply(fmt.Sprintf("Warning threshold for this chat set to %d warnings.", num))
}

func sendWarnMenu(ctx *Context, s *sqlstore.SQLStore) error {
	chatKey := ctx.Chat.String()
	limitKey := fmt.Sprintf("warn_limit:%s", chatKey)
	rawLimit, _ := s.GetSetting(ctx.Ctx, limitKey)
	maxLimit, _ := strconv.Atoi(rawLimit)
	if maxLimit <= 0 {
		maxLimit = 3
	}

	p := ctx.GetPrefix()
	bodyText := fmt.Sprintf("╭━━━〔 WARN CONFIGURATION 〕━━━\n│ Max Warn Threshold : %d Warnings\n╰━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\nChoose an option below to set threshold or view customization guide.", maxLimit)

	buttons := []struct{ ID, Text string }{
		{ID: p + "setwarn 3", Text: "Set Limit (3)"},
		{ID: p + "warn customize", Text: "Customize"},
	}

	return sendInteractiveButtons(ctx, bodyText, fmt.Sprintf("%s Warn Moderation", ctx.GetBotName()), buttons)
}

func sendWarnCustomizeGuide(ctx *Context) error {
	p := ctx.GetPrefix()
	var sb strings.Builder
	sb.WriteString("╭━━━〔 WARN CUSTOMIZATION GUIDE 〕━━━\n\n")
	sb.WriteString("Available Commands:\n")
	fmt.Fprintf(&sb, "• Issue Warning     : `%swarn @user [reason]`\n", p)
	fmt.Fprintf(&sb, "• Remove Warning    : `%sunwarn @user`\n", p)
	fmt.Fprintf(&sb, "• Check Warnings    : `%swarns [@user]`\n", p)
	fmt.Fprintf(&sb, "• Set Max Threshold : `%ssetwarn <number>`\n\n", p)

	sb.WriteString("Automated Enforcement Rules:\n")
	sb.WriteString("1. Reaching max threshold in Group -> Blocks user & Kicks from group (requires bot admin).\n")
	sb.WriteString("2. Reaching max threshold in Private Chat -> Blocks send.\n")
	sb.WriteString("3. Bot Owner & Sudoers are immune to warnings.\n")
	sb.WriteString("4. Group Admins cannot be kicked unless bot is group owner.\n\n")

	sb.WriteString("Examples:\n")
	fmt.Fprintf(&sb, "1. `%swarn @user Spamming links in group`\n", p)
	fmt.Fprintf(&sb, "2. `%sunwarn @user`\n", p)
	fmt.Fprintf(&sb, "3. `%ssetwarn 3`\n", p)

	return ctx.Reply(strings.TrimSpace(sb.String()))
}

func extractWarnTarget(ctx *Context, args []string) types.JID {
	if quotedSender, ok := ctx.GetQuotedSender(); ok && !quotedSender.IsEmpty() {
		return NormalizeUserJID(ctx.Ctx, ctx.Client, quotedSender)
	}
	if ci := ctx.GetContextInfo(); ci != nil && len(ci.GetMentionedJID()) > 0 {
		for _, m := range ci.GetMentionedJID() {
			if parsed, err := send.ParseUserJID(m); err == nil && !parsed.IsEmpty() {
				return NormalizeUserJID(ctx.Ctx, ctx.Client, parsed)
			}
		}
	}
	for _, arg := range args {
		sub := strings.ToLower(arg)
		if sub == "customize" || sub == "custom" || sub == "help" || sub == "limit" || sub == "max" || sub == "set" {
			continue
		}
		if _, err := strconv.Atoi(arg); err == nil {
			continue
		}
		if parsed, err := send.ParseUserJID(arg); err == nil && !parsed.IsEmpty() {
			return NormalizeUserJID(ctx.Ctx, ctx.Client, parsed)
		}
	}
	return types.EmptyJID
}

func isJIDOwnerOrSudo(ctx *Context, target types.JID) bool {
	if ctx.Client.Store.ID != nil && ctx.IsSameUser(target, *ctx.Client.Store.ID) {
		return true
	}
	return isJIDSudo(ctx, target)
}

func isParticipantAdmin(info *types.GroupInfo, target types.JID) bool {
	if info == nil {
		return false
	}
	targetUser := target.ToNonAD().User
	for _, p := range info.Participants {
		if (p.JID.User == targetUser) && (p.IsAdmin || p.IsSuperAdmin) {
			return true
		}
	}
	return false
}

func isBotAdmin(ctx *Context, info *types.GroupInfo) bool {
	if info == nil || ctx.Client.Store.ID == nil {
		return false
	}
	botUser := ctx.Client.Store.ID.ToNonAD().User
	botLIDUser := ""
	if !ctx.Client.Store.LID.IsEmpty() {
		botLIDUser = ctx.Client.Store.LID.ToNonAD().User
	}

	for _, p := range info.Participants {
		if (p.JID.User == botUser || (botLIDUser != "" && p.JID.User == botLIDUser)) && (p.IsAdmin || p.IsSuperAdmin) {
			return true
		}
	}
	return false
}

func isBotGroupOwner(ctx *Context, info *types.GroupInfo) bool {
	if info == nil || info.OwnerJID.IsEmpty() || ctx.Client.Store.ID == nil {
		return false
	}
	return ctx.IsSameUser(info.OwnerJID, *ctx.Client.Store.ID)
}
