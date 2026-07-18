package commands

import (
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/Thruqe/whatsrook/store/sqlstore"
)

func init() {
	Register(&Command{
		Name:        "setcmd",
		Description: "Link a sticker to a command trigger. Usage: setcmd [command_name] (replying to a sticker)",
		Category:    "settings",
		Handler:     handleSetCmd,
	})
	Register(&Command{
		Name:        "delcmd",
		Description: "Unlink a sticker from a command trigger. Usage: delcmd [command_name] or reply to a mapped sticker",
		Category:    "settings",
		Handler:     handleDelCmd,
	})
	Register(&Command{
		Name:        "getcmd",
		Description: "List all mapped sticker commands",
		Category:    "settings",
		Handler:     handleGetCmd,
	})
}

func handleSetCmd(ctx *Context) error {
	if !ctx.IsSudo() {
		return ctx.Reply("❌ You are not authorized to use this command.")
	}

	if len(ctx.Args) == 0 {
		return ctx.Reply("❌ Usage: setcmd [command_name] (reply to a sticker)")
	}
	cmdName := strings.ToLower(ctx.Args[0])

	// Ensure the command exists
	_, exists := Get(cmdName)
	if !exists {
		return ctx.Reply(fmt.Sprintf("❌ Command %q does not exist.", cmdName))
	}

	quoted := ctx.GetQuotedMessage()
	if quoted == nil || quoted.StickerMessage == nil {
		return ctx.Reply("❌ Please reply to a sticker message.")
	}

	stk := quoted.StickerMessage
	if len(stk.FileSHA256) == 0 {
		return ctx.Reply("❌ Invalid sticker (no FileSHA256 found).")
	}

	s, ok := ctx.Client.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return ctx.Reply("❌ Settings store unavailable.")
	}
	db := s.GetDB()
	if db == nil {
		return ctx.Reply("❌ Database unavailable.")
	}

	ourJID := ctx.Client.Store.ID.ToNonAD().String()
	shaHex := hex.EncodeToString(stk.FileSHA256)

	_, err := db.Exec(ctx.Ctx, `
		INSERT INTO bot_sticker_cmds (our_jid, sticker_sha256, command_name)
		VALUES ($1, $2, $3)
		ON CONFLICT(our_jid, sticker_sha256) DO UPDATE SET command_name=excluded.command_name
	`, ourJID, shaHex, cmdName)
	if err != nil {
		return err
	}

	return ctx.Reply(fmt.Sprintf("✅ Sticker linked to command %q.", cmdName))
}

func handleDelCmd(ctx *Context) error {
	if !ctx.IsSudo() {
		return ctx.Reply("❌ You are not authorized to use this command.")
	}

	s, ok := ctx.Client.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return ctx.Reply("❌ Settings store unavailable.")
	}
	db := s.GetDB()
	if db == nil {
		return ctx.Reply("❌ Database unavailable.")
	}

	ourJID := ctx.Client.Store.ID.ToNonAD().String()

	quoted := ctx.GetQuotedMessage()
	if quoted != nil && quoted.StickerMessage != nil {
		stk := quoted.StickerMessage
		if len(stk.FileSHA256) == 0 {
			return ctx.Reply("❌ Invalid sticker (no FileSHA256 found).")
		}
		shaHex := hex.EncodeToString(stk.FileSHA256)

		res, err := db.Exec(ctx.Ctx, `DELETE FROM bot_sticker_cmds WHERE our_jid=$1 AND sticker_sha256=$2`, ourJID, shaHex)
		if err != nil {
			return err
		}
		rows, _ := res.RowsAffected()
		if rows == 0 {
			return ctx.Reply("ℹ️ Mapped sticker not found.")
		}
		return ctx.Reply("✅ Sticker link removed.")
	}

	if len(ctx.Args) == 0 {
		return ctx.Reply("❌ Usage:\n- delcmd [command_name]\n- delcmd (replying to a mapped sticker)")
	}

	cmdName := strings.ToLower(ctx.Args[0])
	res, err := db.Exec(ctx.Ctx, `DELETE FROM bot_sticker_cmds WHERE our_jid=$1 AND command_name=$2`, ourJID, cmdName)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ctx.Reply(fmt.Sprintf("ℹ️ No sticker linked to command %q.", cmdName))
	}

	return ctx.Reply(fmt.Sprintf("✅ Mapped sticker(s) for command %q removed.", cmdName))
}

func handleGetCmd(ctx *Context) error {
	s, ok := ctx.Client.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return ctx.Reply("❌ Settings store unavailable.")
	}
	db := s.GetDB()
	if db == nil {
		return ctx.Reply("❌ Database unavailable.")
	}

	ourJID := ctx.Client.Store.ID.ToNonAD().String()

	rows, err := db.Query(ctx.Ctx, `SELECT sticker_sha256, command_name FROM bot_sticker_cmds WHERE our_jid=$1`, ourJID)
	if err != nil {
		return err
	}
	defer rows.Close()

	var sb strings.Builder
	sb.WriteString("🎨 *Sticker Command Mappings:*\n\n")

	count := 0
	for rows.Next() {
		var sha, cmdName string
		if err := rows.Scan(&sha, &cmdName); err == nil {
			fmt.Fprintf(&sb, "- %s -> %s\n", sha[:8]+"...", cmdName)
			count++
		}
	}

	if count == 0 {
		return ctx.Reply("ℹ️ No sticker commands configured.")
	}

	return ctx.Reply(sb.String())
}
