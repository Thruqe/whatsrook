// Ping command – customizable response format & latency checker.
package plugins

import (
	"fmt"
	"strings"
	"time"

	"whatsrook/wa-core/store/sqlstore"
)

const (
	PingTemplateKey = "ping_template"
	defaultPingTpl  = "🏓 *Pong!*\n\n• Start  : `{start}`\n• End    : `{end}`\n• Latency: `{latency}`\n• Lag    : `{lag}`"
)

func init() {
	Register(&Command{
		Name:        "ping",
		Description: "Check if the bot is alive and measure response latency (customizable with .ping set <template>)",
		Category:    "info",
		IsPublic:    true,
		Handler:     handlePing,
	})
}

func handlePing(ctx *Context) error {
	s, okStore := ctx.Client.Store.Identities.(*sqlstore.SQLStore)

	args := strings.Fields(ctx.RawArgs)
	if len(args) > 0 {
		sub := strings.ToLower(args[0])
		if (sub == "set" || sub == "msg" || sub == "template" || sub == "custom") && okStore {
			if !ctx.IsSudo() {
				return ctx.Reply("Only sudoers/owners can customize the ping template.")
			}
			if len(args) < 2 {
				curr, _ := s.GetSetting(ctx.Ctx, PingTemplateKey)
				if curr == "" {
					curr = defaultPingTpl
				}
				return ctx.Reply("Current Ping Template:\n\n" + curr)
			}
			newTpl := strings.TrimSpace(ctx.RawArgs[len(args[0]):])
			if strings.EqualFold(newTpl, "reset") || strings.EqualFold(newTpl, "clear") {
				_ = s.PutSetting(ctx.Ctx, PingTemplateKey, "")
				return ctx.Reply("Ping template reset to default.")
			}
			if err := s.PutSetting(ctx.Ctx, PingTemplateKey, newTpl); err != nil {
				return ctx.Reply("Failed to save ping template: " + err.Error())
			}
			return ctx.Reply("Ping template updated successfully!\n\nUse `" + ctx.GetPrefix() + "ping set reset` to restore default.")
		}
	}

	start := time.Now()
	msgID, err := ctx.ReplyWithID("⚡ Ping...")
	if err != nil {
		return err
	}

	end := time.Now()
	elapsed := end.Sub(start)

	var latency string
	if elapsed < time.Millisecond {
		latency = fmt.Sprintf("%.2f µs", float64(elapsed.Microseconds()))
	} else if elapsed < time.Second {
		latency = fmt.Sprintf("%d ms", elapsed.Milliseconds())
	} else {
		latency = fmt.Sprintf("%.2f s", elapsed.Seconds())
	}

	startTimeStr := start.Format("15:04:05.000")
	endTimeStr := end.Format("15:04:05.000")

	lagStr := "0 ms"
	if ctx.Evt != nil && !ctx.Evt.Info.Timestamp.IsZero() {
		msgLag := start.Sub(ctx.Evt.Info.Timestamp)
		if msgLag > 0 {
			lagStr = fmt.Sprintf("%d ms", msgLag.Milliseconds())
		}
	}

	tpl := defaultPingTpl
	if okStore {
		if custom, err := s.GetSetting(ctx.Ctx, PingTemplateKey); err == nil && custom != "" {
			tpl = custom
		}
	}

	replacer := strings.NewReplacer(
		"{start}", startTimeStr,
		"{end}", endTimeStr,
		"{latency}", latency,
		"{lag}", lagStr,
		"{bot}", ctx.GetBotName(),
	)

	respText := replacer.Replace(tpl)

	_, editErr := ctx.Edit(msgID, respText)
	if editErr != nil {
		_ = ctx.Reply(respText)
	}
	return nil
}
