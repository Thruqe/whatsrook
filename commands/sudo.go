package commands

import (
	"fmt"
	"strings"

	"github.com/Thruqe/whatsrook/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
)

func init() {
	Register(&Command{
		Name:        "setsudo",
		Description: "Add a user to the sudo list (replied user or numbers)",
		Category:    "settings",
		Handler:     handleSetSudo,
	})
	Register(&Command{
		Name:        "delsudo",
		Description: "Remove a user from the sudo list (replied user or numbers)",
		Category:    "settings",
		Handler:     handleDelSudo,
	})
	Register(&Command{
		Name:        "listsudo",
		Description: "List all sudo users",
		Category:    "settings",
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
		Description: "Toggle automatic ViewOnce message forwarding to DM (on/off)",
		Category:    "settings",
		Handler:     handleAutoVV,
	})
	Register(&Command{
		Name:        "autostatussave",
		Description: "Toggle automatic status updates saving to DM (on/off)",
		Category:    "settings",
		Handler:     handleAutoStatusSave,
	})
	Register(&Command{
		Name:        "ban",
		Description: "Block a user from using the bot commands (replied user or numbers)",
		Category:    "settings",
		Handler:     handleBan,
	})
	Register(&Command{
		Name:        "unban",
		Description: "Unblock a user (replied user or numbers)",
		Category:    "settings",
		Handler:     handleUnban,
	})
	Register(&Command{
		Name:        "mode",
		Description: "Toggle bot mode (public/private)",
		Category:    "settings",
		Handler:     handleMode,
	})
}

func handleSetSudo(ctx *Context) error {
	if !ctx.IsSudo() {
		return ctx.Reply("❌ You are not authorized to use this command.")
	}

	targets := ctx.GetTargets()
	if len(targets) == 0 {
		return ctx.Reply("❌ Please reply to a user, tag them, or type their phone number.")
	}

	s, ok := ctx.Client.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return ctx.Reply("❌ Settings store unavailable.")
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
		already := false
		for _, sdr := range sudoers {
			if sdr == targetStr {
				already = true
				break
			}
		}
		if !already {
			sudoers = append(sudoers, targetStr)
			resolvedJID, username := ctx.ResolveMention(target)
			addedJIDs = append(addedJIDs, resolvedJID)
			displayNames = append(displayNames, "@"+username)
		}
	}

	if len(addedJIDs) == 0 {
		return ctx.Reply("ℹ️ Target(s) already in the sudo list.")
	}

	if err := s.PutSetting(ctx.Ctx, "sudoers", strings.Join(sudoers, " ")); err != nil {
		return err
	}

	return ctx.ReplyWithMentions(fmt.Sprintf("✅ Added to sudo: %s", strings.Join(displayNames, ", ")), addedJIDs)
}

func handleDelSudo(ctx *Context) error {
	if !ctx.IsSudo() {
		return ctx.Reply("❌ You are not authorized to use this command.")
	}

	targets := ctx.GetTargets()
	if len(targets) == 0 {
		return ctx.Reply("❌ Please reply to a user, tag them, or type their phone number.")
	}

	s, ok := ctx.Client.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return ctx.Reply("❌ Settings store unavailable.")
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
		return ctx.Reply("ℹ️ Target(s) not found in the sudo list.")
	}

	if err := s.PutSetting(ctx.Ctx, "sudoers", strings.Join(newSudoers, " ")); err != nil {
		return err
	}

	return ctx.ReplyWithMentions(fmt.Sprintf("✅ Removed from sudo: %s", strings.Join(displayNames, ", ")), removedJIDs)
}

func handleListSudo(ctx *Context) error {
	if !ctx.IsSudo() {
		return ctx.Reply("❌ You are not authorized to use this command.")
	}

	s, ok := ctx.Client.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return ctx.Reply("❌ Settings store unavailable.")
	}

	raw, err := s.GetSetting(ctx.Ctx, "sudoers")
	if err != nil {
		return err
	}

	sudoers := strings.Fields(raw)
	var mentions []types.JID
	var sb strings.Builder
	sb.WriteString("👑 *Sudoers List*\n\n")

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
		return ctx.Reply("❌ You are not authorized to use this command.")
	}

	if len(ctx.Args) == 0 {
		return ctx.Reply("❌ Usage: disablecmd <command_name>")
	}

	cmdName := strings.ToLower(ctx.Args[0])
	if cmdName == "enablecmd" || cmdName == "disablecmd" {
		return ctx.Reply("❌ Cannot disable core system commands.")
	}

	_, exists := Get(cmdName)
	if !exists {
		return ctx.Reply(fmt.Sprintf("❌ Command %q does not exist.", cmdName))
	}

	s, ok := ctx.Client.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return ctx.Reply("❌ Settings store unavailable.")
	}

	raw, err := s.GetSetting(ctx.Ctx, "disabled_commands")
	if err != nil {
		return err
	}

	disabled := strings.Fields(raw)
	for _, d := range disabled {
		if strings.EqualFold(d, cmdName) {
			return ctx.Reply(fmt.Sprintf("ℹ️ Command %q is already disabled.", cmdName))
		}
	}

	disabled = append(disabled, cmdName)
	if err := s.PutSetting(ctx.Ctx, "disabled_commands", strings.Join(disabled, " ")); err != nil {
		return err
	}

	return ctx.Reply(fmt.Sprintf("✅ Command %q has been disabled.", cmdName))
}

func handleEnableCmd(ctx *Context) error {
	if !ctx.IsSudo() {
		return ctx.Reply("❌ You are not authorized to use this command.")
	}

	if len(ctx.Args) == 0 {
		return ctx.Reply("❌ Usage: enablecmd <command_name>")
	}

	cmdName := strings.ToLower(ctx.Args[0])
	s, ok := ctx.Client.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return ctx.Reply("❌ Settings store unavailable.")
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
		return ctx.Reply(fmt.Sprintf("ℹ️ Command %q is not currently disabled.", cmdName))
	}

	if err := s.PutSetting(ctx.Ctx, "disabled_commands", strings.Join(newDisabled, " ")); err != nil {
		return err
	}

	return ctx.Reply(fmt.Sprintf("✅ Command %q has been enabled.", cmdName))
}

func handleAutoVV(ctx *Context) error {
	if !ctx.IsSudo() {
		return ctx.Reply("❌ You are not authorized to use this command.")
	}

	if len(ctx.Args) == 0 {
		return ctx.Reply("❌ Usage: autovv [on/off]")
	}

	state := strings.ToLower(ctx.Args[0])
	if state != "on" && state != "off" {
		return ctx.Reply("❌ Invalid state. Usage: autovv [on/off]")
	}

	s, ok := ctx.Client.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return ctx.Reply("❌ Settings store unavailable.")
	}

	err := s.PutSetting(ctx.Ctx, "autovv", state)
	if err != nil {
		return err
	}

	if state == "on" {
		return ctx.Reply("✅ Auto ViewOnce forwarding enabled.\n\n⚠️ Note: This feature only works if the client connection type is set to Android or iOS (web clients do not receive ViewOnce media).")
	}

	return ctx.Reply("✅ Auto ViewOnce forwarding disabled.")
}

func handleAutoStatusSave(ctx *Context) error {
	if !ctx.IsSudo() {
		return ctx.Reply("❌ You are not authorized to use this command.")
	}

	if len(ctx.Args) == 0 {
		return ctx.Reply("❌ Usage: autostatussave [on/off]")
	}

	state := strings.ToLower(ctx.Args[0])
	if state != "on" && state != "off" {
		return ctx.Reply("❌ Invalid state. Usage: autostatussave [on/off]")
	}

	s, ok := ctx.Client.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return ctx.Reply("❌ Settings store unavailable.")
	}

	err := s.PutSetting(ctx.Ctx, "autostatussave", state)
	if err != nil {
		return err
	}

	if state == "on" {
		return ctx.Reply("✅ Auto Status saving enabled. Status updates will now be automatically sent to your DM.")
	}

	return ctx.Reply("✅ Auto Status saving disabled.")
}

func handleBan(ctx *Context) error {
	if !ctx.IsSudo() {
		return ctx.Reply("❌ You are not authorized to use this command.")
	}

	targets := ctx.GetTargets()
	if len(targets) == 0 {
		return ctx.Reply("❌ Please reply to a user, tag them, or type their phone number.")
	}

	s, ok := ctx.Client.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return ctx.Reply("❌ Settings store unavailable.")
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

		already := false
		for _, b := range bannedUsers {
			if b == targetStr {
				already = true
				break
			}
		}

		if !already {
			bannedUsers = append(bannedUsers, targetStr)
			bannedJIDs = append(bannedJIDs, target)

			displayJID := target
			if target.Server == types.HiddenUserServer && ctx.Client.Store.LIDs != nil {
				if pn, err := ctx.Client.Store.LIDs.GetPNForLID(ctx.Ctx, target); err == nil && !pn.IsEmpty() {
					displayJID = pn.ToNonAD()
				}
			}
			displayNames = append(displayNames, "@"+displayJID.User)
		}
	}

	if len(bannedJIDs) == 0 {
		return ctx.Reply("ℹ️ Target(s) could not be banned (already banned, owner, or sudo).")
	}

	if err := s.PutSetting(ctx.Ctx, "banned_users", strings.Join(bannedUsers, " ")); err != nil {
		return err
	}

	return ctx.ReplyWithMentions(fmt.Sprintf("✅ Banned from commands: %s", strings.Join(displayNames, ", ")), bannedJIDs)
}

func handleUnban(ctx *Context) error {
	if !ctx.IsSudo() {
		return ctx.Reply("❌ You are not authorized to use this command.")
	}

	targets := ctx.GetTargets()
	if len(targets) == 0 {
		return ctx.Reply("❌ Please reply to a user, tag them, or type their phone number.")
	}

	s, ok := ctx.Client.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return ctx.Reply("❌ Settings store unavailable.")
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
				unbannedJIDs = append(unbannedJIDs, target)

				displayJID := target
				if target.Server == types.HiddenUserServer && ctx.Client.Store.LIDs != nil {
					if pn, err := ctx.Client.Store.LIDs.GetPNForLID(ctx.Ctx, target); err == nil && !pn.IsEmpty() {
						displayJID = pn.ToNonAD()
					}
				}
				displayNames = append(displayNames, "@"+displayJID.User)
				break
			}
		}
		if !matched {
			newBanned = append(newBanned, b)
		}
	}

	if len(unbannedJIDs) == 0 {
		return ctx.Reply("ℹ️ Target(s) not found in the banned list.")
	}

	if err := s.PutSetting(ctx.Ctx, "banned_users", strings.Join(newBanned, " ")); err != nil {
		return err
	}

	return ctx.ReplyWithMentions(fmt.Sprintf("✅ Unbanned from commands: %s", strings.Join(displayNames, ", ")), unbannedJIDs)
}

func handleMode(ctx *Context) error {
	if !ctx.IsSudo() {
		return ctx.Reply("❌ You are not authorized to use this command.")
	}

	s, ok := ctx.Client.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return ctx.Reply("❌ Settings store unavailable.")
	}

	if len(ctx.Args) == 0 {
		current, err := s.GetSetting(ctx.Ctx, "mode")
		if err != nil {
			return err
		}
		if current == "" {
			current = "public"
		}
		return ctx.Reply(fmt.Sprintf("ℹ️ The bot is currently in %s mode.", current))
	}

	mode := strings.ToLower(ctx.Args[0])
	if mode != "public" && mode != "private" {
		return ctx.Reply("❌ Invalid mode. Usage: mode [public/private]")
	}

	err := s.PutSetting(ctx.Ctx, "mode", mode)
	if err != nil {
		return err
	}

	return ctx.Reply(fmt.Sprintf("✅ Bot mode set to %s.", mode))
}
