// Sudo user management – add/remove/list sudo users who have elevated access.
package plugins

import (
	"fmt"
	"slices"
	"strings"

	"whatsrook/store/sqlstore"

	"go.mau.fi/whatsmeow/types"
)

func init() {
	Register(&Command{
		Name:        "setsudo",
		Description: "Add a user to the sudo list (replied user or numbers)",
		Category:    "owner",
		Handler:     handleSetSudo,
	})
	Register(&Command{
		Name:        "delsudo",
		Description: "Remove a user from the sudo list (replied user or numbers)",
		Category:    "owner",
		Handler:     handleDelSudo,
	})
	Register(&Command{
		Name:        "listsudo",
		Description: "List all sudo users",
		Category:    "owner",
		Handler:     handleListSudo,
	})
	Register(&Command{
		Name:        "disablecmd",
		Description: "Disable a command globally for normal users",
		Category:    "settings",
		Handler:     handleDisableCmd,
	})
	Register(&Command{
		Name:        "enablecmd",
		Description: "Enable a previously disabled command",
		Category:    "settings",
		Handler:     handleEnableCmd,
	})
	Register(&Command{
		Name:        "autovv",
		Aliases:     []string{"vvauto"},
		Description: "Toggle automatic ViewOnce message forwarding to DM",
		Category:    "settings",
		Handler:     handleAutoVV,
	})
	Register(&Command{
		Name:        "autostatus",
		Aliases:     []string{"statussave", "statusauto", "autostatussave"},
		Description: "Toggle automatic status updates saving to DM",
		Category:    "settings",
		Handler:     handleAutoStatusSave,
	})
	Register(&Command{
		Name:        "ban",
		Description: "Block a user from using the bot commands (replied user or numbers)",
		Category:    "owner",
		Handler:     handleBan,
	})
	Register(&Command{
		Name:        "unban",
		Description: "Unblock a user (replied user or numbers)",
		Category:    "owner",
		Handler:     handleUnban,
	})
	Register(&Command{
		Name:        "mode",
		Description: "Toggle bot mode (public/private)",
		Category:    "owner",
		Handler:     handleMode,
	})
}

func handleSetSudo(ctx *Context) error {
	if !ctx.IsSudo() {
		return ctx.Reply("You are not authorized to use this command.")
	}

	targets := ctx.GetTargets()
	if len(targets) == 0 {
		p := ctx.GetPrefix()
		return ctx.Reply(fmt.Sprintf("Usage:\n- %ssetsudo @user\n- %ssetsudo 1234567890\n- Reply to a user's message with %ssetsudo", p, p, p))
	}

	s, ok := ctx.Client.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return ctx.Reply("Settings store unavailable.")
	}

	raw, err := s.GetSetting(ctx.Ctx, "sudoers")
	if err != nil {
		return err
	}

	sudoers := strings.Fields(raw)
	var addedJIDs []types.JID
	var displayNames []string

	for _, target := range targets {
		targetStr := target.ToNonAD().String()
		already := slices.Contains(sudoers, targetStr)
		if !already {
			sudoers = append(sudoers, targetStr)
			resolvedJID, username := ctx.ResolveMention(target)
			addedJIDs = append(addedJIDs, resolvedJID)
			displayNames = append(displayNames, "@"+username)
		}
	}

	if len(addedJIDs) == 0 {
		return ctx.Reply("Target(s) already in the sudo list.")
	}

	if err := s.PutSetting(ctx.Ctx, "sudoers", strings.Join(sudoers, " ")); err != nil {
		return ctx.Reply("Failed to update sudoers list.")
	}

	return ctx.ReplyWithMentions(fmt.Sprintf("Added to sudo: %s", strings.Join(displayNames, ", ")), addedJIDs)
}

func handleDelSudo(ctx *Context) error {
	if !ctx.IsSudo() {
		return ctx.Reply("You are not authorized to use this command.")
	}
	if !ctx.IsOwner() {
		return ctx.Reply("Only the bot owner can remove users from the sudo list.")
	}

	targets := ctx.GetTargets()
	if len(targets) == 0 {
		p := ctx.GetPrefix()
		return ctx.Reply(fmt.Sprintf("Usage:\n- %sdelsudo @user\n- %sdelsudo 1234567890\n- Reply to a user's message with %sdelsudo", p, p, p))
	}

	s, ok := ctx.Client.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return ctx.Reply("Settings store unavailable.")
	}

	raw, err := s.GetSetting(ctx.Ctx, "sudoers")
	if err != nil {
		return err
	}

	sudoers := strings.Fields(raw)
	var removedJIDs []types.JID
	var displayNames []string
	newSudoers := []string{}

	for _, sdr := range sudoers {
		matched := false
		for _, target := range targets {
			if sdr == target.ToNonAD().String() {
				matched = true
				resolvedJID, username := ctx.ResolveMention(target)
				removedJIDs = append(removedJIDs, resolvedJID)
				displayNames = append(displayNames, "@"+username)
				break
			}
		}
		if !matched {
			newSudoers = append(newSudoers, sdr)
		}
	}

	if len(removedJIDs) == 0 {
		return ctx.Reply("Target(s) not found in the sudo list.")
	}

	if err := s.PutSetting(ctx.Ctx, "sudoers", strings.Join(newSudoers, " ")); err != nil {
		return ctx.Reply("Failed to update sudoers list.")
	}

	return ctx.ReplyWithMentions(fmt.Sprintf("Removed from sudo: %s", strings.Join(displayNames, ", ")), removedJIDs)
}

func handleListSudo(ctx *Context) error {
	if !ctx.IsSudo() {
		return ctx.Reply("You are not authorized to use this command.")
	}

	s, ok := ctx.Client.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return ctx.Reply("Settings store unavailable.")
	}

	raw, err := s.GetSetting(ctx.Ctx, "sudoers")
	if err != nil {
		return err
	}

	sudoers := strings.Fields(raw)
	var mentions []types.JID
	var sb strings.Builder
	sb.WriteString("Sudo List\n\n")

	if ctx.Client.Store.ID != nil {
		ownerJID := ctx.Client.Store.ID.ToNonAD()
		resolvedJID, username := ctx.ResolveMention(ownerJID)
		fmt.Fprintf(&sb, "- @%s (Owner)\n", username)
		mentions = append(mentions, resolvedJID)
	}

	for _, sdr := range sudoers {
		sudoerJID, err := types.ParseJID(sdr)
		if err == nil {
			sudoerJID = sudoerJID.ToNonAD()
			if ctx.Client.Store.ID != nil && ctx.IsSameUser(sudoerJID, *ctx.Client.Store.ID) {
				continue
			}
			resolvedJID, username := ctx.ResolveMention(sudoerJID)
			fmt.Fprintf(&sb, "- @%s\n", username)
			mentions = append(mentions, resolvedJID)
		}
	}

	return ctx.ReplyWithMentions(sb.String(), mentions)
}

func handleDisableCmd(ctx *Context) error {
	if !ctx.IsSudo() {
		return ctx.Reply("You are not authorized to use this command.")
	}

	if len(ctx.Args) == 0 {
		p := ctx.GetPrefix()
		return ctx.Reply(fmt.Sprintf("Usage:\n- %sdisablecmd <command_name>\nExample:\n- %sdisablecmd weather", p, p))
	}

	cmdName := strings.ToLower(ctx.Args[0])
	if cmdName == "enablecmd" || cmdName == "disablecmd" {
		return ctx.Reply("Cannot disable core system commands.")
	}

	_, exists := Get(cmdName)
	if !exists {
		return ctx.Reply(fmt.Sprintf("Command %q does not exist.", cmdName))
	}

	s, ok := ctx.Client.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return ctx.Reply("Settings store unavailable.")
	}

	raw, err := s.GetSetting(ctx.Ctx, "disabled_commands")
	if err != nil {
		return ctx.Reply("Failed to retrieve disabled commands setting.")
	}

	disabled := strings.Fields(raw)
	for _, d := range disabled {
		if strings.EqualFold(d, cmdName) {
			return ctx.Reply(fmt.Sprintf("Command %q is already disabled.", cmdName))
		}
	}

	disabled = append(disabled, cmdName)
	if err := s.PutSetting(ctx.Ctx, "disabled_commands", strings.Join(disabled, " ")); err != nil {
		return ctx.Reply("Failed to disable command.")
	}

	return ctx.Reply(fmt.Sprintf("Command %q has been disabled.", cmdName))
}

func handleEnableCmd(ctx *Context) error {
	if !ctx.IsSudo() {
		return ctx.Reply("You are not authorized to use this command.")
	}

	if len(ctx.Args) == 0 {
		p := ctx.GetPrefix()
		return ctx.Reply(fmt.Sprintf("Usage:\n- %senablecmd <command_name>\nExample:\n- %senablecmd weather", p, p))
	}

	cmdName := strings.ToLower(ctx.Args[0])
	s, ok := ctx.Client.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return ctx.Reply("Settings store unavailable.")
	}

	raw, err := s.GetSetting(ctx.Ctx, "disabled_commands")
	if err != nil {
		return err
	}

	disabled := strings.Fields(raw)
	found := false
	newDisabled := []string{}

	for _, d := range disabled {
		if strings.EqualFold(d, cmdName) {
			found = true
		} else {
			newDisabled = append(newDisabled, d)
		}
	}

	if !found {
		return ctx.Reply(fmt.Sprintf("Command %q is not currently disabled.", cmdName))
	}

	if err := s.PutSetting(ctx.Ctx, "disabled_commands", strings.Join(newDisabled, " ")); err != nil {
		return ctx.Reply("Failed to enable command.")
	}

	return ctx.Reply(fmt.Sprintf("Command %q has been enabled.", cmdName))
}

func handleAutoVV(ctx *Context) error {
	if !ctx.IsSudo() {
		return ctx.Reply("You are not authorized to use this command.")
	}

	s, ok := ctx.Client.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return ctx.Reply("Settings store unavailable.")
	}

	args := strings.Fields(ctx.RawArgs)
	if len(args) == 0 {
		curr, _ := s.GetSetting(ctx.Ctx, "autovv")
		if curr == "" {
			curr = "off"
		}
		mode, _ := s.GetSetting(ctx.Ctx, "autovv_mode")
		if mode == "" {
			mode = "dm"
		}
		p := ctx.GetPrefix()
		bodyText := fmt.Sprintf("╭━━━〔 AUTO-VIEWONCE FORWARDING 〕━━━\n│ Status : %s\n│ Mode   : %s\n╰━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\nAutomatically forwards unwrapped ViewOnce media to your DM or directly in the chat.", strings.ToUpper(curr), strings.ToUpper(mode))
		var actionButton struct{ ID, Text string }
		if curr == "on" {
			actionButton = struct{ ID, Text string }{ID: p + "autovv off", Text: "Deactivate"}
		} else {
			actionButton = struct{ ID, Text string }{ID: p + "autovv on", Text: "Activate"}
		}
		var modeButton struct{ ID, Text string }
		if mode == "public" {
			modeButton = struct{ ID, Text string }{ID: p + "autovv dm", Text: "Switch to DM"}
		} else {
			modeButton = struct{ ID, Text string }{ID: p + "autovv public", Text: "Switch to Public"}
		}
		buttons := []struct{ ID, Text string }{
			actionButton,
			modeButton,
			{ID: p + "autovv customize", Text: "Customize"},
		}
		return sendInteractiveButtons(ctx, bodyText, fmt.Sprintf("%s AutoVV", ctx.GetBotName()), buttons)
	}

	sub := strings.ToLower(args[0])
	switch sub {
	case "on", "enable":
		_ = s.PutSetting(ctx.Ctx, "autovv", "on")
		return ctx.Reply("Auto ViewOnce forwarding ENABLED.\n\nNote: This feature requires Android or iOS client mode.")
	case "off", "disable":
		_ = s.PutSetting(ctx.Ctx, "autovv", "off")
		return ctx.Reply("Auto ViewOnce forwarding DISABLED.")
	case "dm", "private":
		_ = s.PutSetting(ctx.Ctx, "autovv_mode", "dm")
		return ctx.Reply("Auto ViewOnce delivery mode set to DM (Private).")
	case "public", "chat":
		_ = s.PutSetting(ctx.Ctx, "autovv_mode", "public")
		return ctx.Reply("Auto ViewOnce delivery mode set to PUBLIC (Same Chat).")
	case "toggle":
		curr, _ := s.GetSetting(ctx.Ctx, "autovv")
		if curr == "on" {
			_ = s.PutSetting(ctx.Ctx, "autovv", "off")
			return ctx.Reply("Auto ViewOnce forwarding DISABLED.")
		}
		_ = s.PutSetting(ctx.Ctx, "autovv", "on")
		return ctx.Reply("Auto ViewOnce forwarding ENABLED.")
	case "customize", "custom", "help":
		p := ctx.GetPrefix()
		return ctx.Reply(fmt.Sprintf("╭━━━〔 AUTOVV GUIDE 〕━━━\n\nCommands:\n• %sautovv on\n• %sautovv off\n• %sautovv dm\n• %sautovv public\n• %sautovv toggle\n\nAutomatically intercepts ViewOnce media sent in chats and forwards unwrapped media to your DM or the public chat.", p, p, p, p, p))
	default:
		return ctx.Reply("Usage: .autovv [on|off|dm|public|toggle|customize]")
	}
}

func handleAutoStatusSave(ctx *Context) error {
	if !ctx.IsSudo() {
		return ctx.Reply("You are not authorized to use this command.")
	}

	s, ok := ctx.Client.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return ctx.Reply("Settings store unavailable.")
	}

	args := strings.Fields(ctx.RawArgs)
	if len(args) == 0 {
		curr, _ := s.GetSetting(ctx.Ctx, "autostatussave")
		if curr == "" {
			curr = "off"
		}
		p := ctx.GetPrefix()
		bodyText := fmt.Sprintf("╭━━━〔 AUTO-STATUS SAVER 〕━━━\n│ Status : %s\n╰━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\nAutomatically saves incoming WhatsApp status broadcasts to your DM.", strings.ToUpper(curr))
		var actionButton struct{ ID, Text string }
		if curr == "on" {
			actionButton = struct{ ID, Text string }{ID: p + "autostatus off", Text: "Deactivate"}
		} else {
			actionButton = struct{ ID, Text string }{ID: p + "autostatus on", Text: "Activate"}
		}
		buttons := []struct{ ID, Text string }{
			actionButton,
			{ID: p + "autostatus customize", Text: "Customize"},
		}
		return sendInteractiveButtons(ctx, bodyText, fmt.Sprintf("%s AutoStatus", ctx.GetBotName()), buttons)
	}

	sub := strings.ToLower(args[0])
	switch sub {
	case "on", "enable":
		_ = s.PutSetting(ctx.Ctx, "autostatussave", "on")
		return ctx.Reply("Auto Status saving ENABLED. incoming status updates will be sent to your DM.")
	case "off", "disable":
		_ = s.PutSetting(ctx.Ctx, "autostatussave", "off")
		return ctx.Reply("Auto Status saving DISABLED.")
	case "toggle":
		curr, _ := s.GetSetting(ctx.Ctx, "autostatussave")
		if curr == "on" {
			_ = s.PutSetting(ctx.Ctx, "autostatussave", "off")
			return ctx.Reply("Auto Status saving DISABLED.")
		}
		_ = s.PutSetting(ctx.Ctx, "autostatussave", "on")
		return ctx.Reply("Auto Status saving ENABLED.")
	case "customize", "custom", "help":
		p := ctx.GetPrefix()
		return ctx.Reply(fmt.Sprintf("╭━━━〔 AUTOSTATUS GUIDE 〕━━━\n\nCommands:\n• %sautostatus on\n• %sautostatus off\n• %sautostatus toggle\n\nAutomatically intercepts contacts' status broadcasts and forwards them to your DM.", p, p, p))
	default:
		return ctx.Reply("Usage: .autostatus [on|off|toggle|customize]")
	}
}

func handleBan(ctx *Context) error {
	if !ctx.IsSudo() {
		return ctx.Reply("You are not authorized to use this command.")
	}

	targets := ctx.GetTargets()
	if len(targets) == 0 {
		p := ctx.GetPrefix()
		return ctx.Reply(fmt.Sprintf("Usage:\n- %sban @user\n- %sban 1234567890\n- Reply to a user's message with %sban", p, p, p))
	}

	s, ok := ctx.Client.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return ctx.Reply("Settings store unavailable.")
	}

	// Sudo list to verify we don't ban a sudo/owner
	rawSudo, _ := s.GetSetting(ctx.Ctx, "sudoers")
	sudoers := strings.Fields(rawSudo)

	rawBanned, err := s.GetSetting(ctx.Ctx, "banned_users")
	if err != nil {
		return err
	}
	bannedUsers := strings.Fields(rawBanned)

	var bannedJIDs []types.JID
	var displayNames []string

	for _, target := range targets {
		targetStr := target.ToNonAD().String()

		// 1. Owner protection
		if ctx.Client.Store.ID != nil {
			if ctx.IsSameUser(target, *ctx.Client.Store.ID) {
				continue
			}
		}

		// 2. Sudo protection
		isSudo := false
		for _, sdr := range sudoers {
			sj, err := types.ParseJID(sdr)
			if err == nil && ctx.IsSameUser(target, sj) {
				isSudo = true
				break
			}
		}
		if isSudo {
			continue // skip sudoers
		}

		already := slices.Contains(bannedUsers, targetStr)

		if !already {
			bannedUsers = append(bannedUsers, targetStr)
			resolvedJID, username := ctx.ResolveMention(target)
			bannedJIDs = append(bannedJIDs, resolvedJID)
			displayNames = append(displayNames, "@"+username)
		}
	}

	if len(bannedJIDs) == 0 {
		return ctx.Reply("Target(s) could not be banned (already banned, owner, or sudo).")
	}

	if err := s.PutSetting(ctx.Ctx, "banned_users", strings.Join(bannedUsers, " ")); err != nil {
		return ctx.Reply("Failed to update banned users list.")
	}

	return ctx.ReplyWithMentions(fmt.Sprintf("Banned from commands: %s", strings.Join(displayNames, ", ")), bannedJIDs)
}

func handleUnban(ctx *Context) error {
	if !ctx.IsSudo() {
		return ctx.Reply("You are not authorized to use this command.")
	}

	targets := ctx.GetTargets()
	if len(targets) == 0 {
		p := ctx.GetPrefix()
		return ctx.Reply(fmt.Sprintf("Usage:\n- %sunban @user\n- %sunban 1234567890\n- Reply to a user's message with %sunban", p, p, p))
	}

	s, ok := ctx.Client.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return ctx.Reply("Settings store unavailable.")
	}

	rawBanned, err := s.GetSetting(ctx.Ctx, "banned_users")
	if err != nil {
		return err
	}
	bannedUsers := strings.Fields(rawBanned)

	var unbannedJIDs []types.JID
	var displayNames []string
	newBanned := []string{}

	for _, b := range bannedUsers {
		matched := false
		for _, target := range targets {
			bj, err := types.ParseJID(b)
			if err == nil && ctx.IsSameUser(target, bj) {
				matched = true
				resolvedJID, username := ctx.ResolveMention(target)
				unbannedJIDs = append(unbannedJIDs, resolvedJID)
				displayNames = append(displayNames, "@"+username)
				break
			}
		}
		if !matched {
			newBanned = append(newBanned, b)
		}
	}

	if len(unbannedJIDs) == 0 {
		return ctx.Reply("Target(s) not found in the banned list.")
	}

	if err := s.PutSetting(ctx.Ctx, "banned_users", strings.Join(newBanned, " ")); err != nil {
		return ctx.Reply("Failed to update banned users list.")
	}

	return ctx.ReplyWithMentions(fmt.Sprintf("Unbanned from commands: %s", strings.Join(displayNames, ", ")), unbannedJIDs)
}

func handleMode(ctx *Context) error {
	if !ctx.IsSudo() {
		return ctx.Reply("You are not authorized to use this command.")
	}

	s, ok := ctx.Client.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return ctx.Reply("Settings store unavailable.")
	}

	p := ctx.GetPrefix()
	if len(ctx.Args) == 0 {
		current, err := s.GetSetting(ctx.Ctx, "mode")
		if err != nil {
			return ctx.Reply("Failed to retrieve bot mode.")
		}
		if current == "" {
			current = "public"
		}
		return ctx.Reply(fmt.Sprintf("Current bot mode: %s\n\nUsage:\n- %smode public\n- %smode private", current, p, p))
	}

	mode := strings.ToLower(ctx.Args[0])
	if mode != "public" && mode != "private" {
		return ctx.Reply("Invalid mode. Usage: mode [public/private]")
	}

	err := s.PutSetting(ctx.Ctx, "mode", mode)
	if err != nil {
		return ctx.Reply("Failed to update bot mode.")
	}

	return ctx.Reply(fmt.Sprintf("Bot mode set to %s.", mode))
}
