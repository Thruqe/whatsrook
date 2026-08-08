// Alive command – displays bot status, uptime, RAM/system stats, and customizable greetings/media.
package plugins

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
	"time"

	"whatsrook/wa-core/store/sqlstore"
)

const (
	AliveTemplateKey = "alive_template"
	AliveMediaKey    = "alive_media"

	defaultAliveTpl = "╭━━━〔 {bot} IS ALIVE 〕━━━\n│ 👤 *Owner*   : {owner}\n│ ⏱️ *Uptime*  : {uptime}\n│ ⚡ *Latency* : {latency}\n│ 🧠 *RAM*     : {ram}\n│ ⚙️ *Goroutines*: {goroutines}\n│ 📌 *Version* : {version}\n╰━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\nType *{prefix}menu* to display available commands."
)

var (
	aliveOnce sync.Once
	bootTime  time.Time
)

func initBootTime() {
	aliveOnce.Do(func() {
		bootTime = time.Now()
	})
}

func init() {
	initBootTime()

	Register(&Command{
		Name:        "alive",
		Aliases:     []string{"botstatus", "system", "uptime"},
		Description: "Check bot online status, uptime, system stats, and custom alive template",
		Category:    "info",
		IsPublic:    true,
		Handler:     handleAlive,
	})
}

func handleAlive(ctx *Context) error {
	initBootTime()
	s, okStore := ctx.Client.Store.Identities.(*sqlstore.SQLStore)

	args := strings.Fields(ctx.RawArgs)
	if len(args) > 0 {
		sub := strings.ToLower(args[0])

		switch sub {
		case "msg", "template", "set":
			if !ctx.IsSudo() {
				return ctx.Reply("Only sudoers/owners can customize the alive message template.")
			}
			if len(args) < 2 {
				curr := defaultAliveTpl
				if okStore {
					if val, err := s.GetSetting(ctx.Ctx, AliveTemplateKey); err == nil && val != "" {
						curr = val
					}
				}
				return ctx.Reply("Current Alive Message Template:\n\n" + curr)
			}
			newTpl := strings.TrimSpace(ctx.RawArgs[len(args[0]):])
			if strings.EqualFold(newTpl, "reset") || strings.EqualFold(newTpl, "clear") {
				if okStore {
					_ = s.PutSetting(ctx.Ctx, AliveTemplateKey, "")
				}
				return ctx.Reply("Alive message template reset to default.")
			}
			if okStore {
				if err := s.PutSetting(ctx.Ctx, AliveTemplateKey, newTpl); err != nil {
					return ctx.Reply("Failed to save alive template: " + err.Error())
				}
			}
			return ctx.Reply("Alive template updated successfully!\n\nUse `" + ctx.GetPrefix() + "alive set reset` to restore default.")

		case "media":
			if !ctx.IsSudo() {
				return ctx.Reply("Only sudoers/owners can set alive media URL.")
			}
			if len(args) < 2 {
				curr := "none"
				if okStore {
					if val, err := s.GetSetting(ctx.Ctx, AliveMediaKey); err == nil && val != "" {
						curr = val
					}
				}
				return ctx.Reply("Current Alive Media URL: " + curr)
			}
			urlVal := strings.TrimSpace(args[1])
			if strings.EqualFold(urlVal, "clear") || strings.EqualFold(urlVal, "off") || strings.EqualFold(urlVal, "none") {
				if okStore {
					_ = s.PutSetting(ctx.Ctx, AliveMediaKey, "")
				}
				return ctx.Reply("Alive media URL cleared.")
			}
			if okStore {
				if err := s.PutSetting(ctx.Ctx, AliveMediaKey, urlVal); err != nil {
					return ctx.Reply("Failed to save alive media: " + err.Error())
				}
			}
			return ctx.Reply("Alive media URL updated successfully!")

		case "customize", "custom", "help":
			return sendAliveCustomizeGuide(ctx)
		}
	}

	startMeasure := time.Now()
	uptime := formatDuration(time.Since(bootTime))

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	ramUsage := fmt.Sprintf("%.2f MB", float64(memStats.Alloc)/1024/1024)
	goroutines := fmt.Sprintf("%d", runtime.NumGoroutine())

	latency := fmt.Sprintf("%d ms", time.Since(startMeasure).Milliseconds())

	ownerName := "Thruqe"
	if ctx.Client != nil && ctx.Client.Store != nil && ctx.Client.Store.ID != nil {
		ownerName = ctx.Client.Store.ID.ToNonAD().User
	}

	tpl := defaultAliveTpl
	mediaURL := ""
	if okStore {
		if val, err := s.GetSetting(ctx.Ctx, AliveTemplateKey); err == nil && val != "" {
			tpl = val
		}
		if mVal, err := s.GetSetting(ctx.Ctx, AliveMediaKey); err == nil && mVal != "" {
			mediaURL = mVal
		}
	}

	replacer := strings.NewReplacer(
		"{bot}", ctx.GetBotName(),
		"{owner}", ownerName,
		"{uptime}", uptime,
		"{latency}", latency,
		"{ram}", ramUsage,
		"{goroutines}", goroutines,
		"{version}", "v2.5.0",
		"{prefix}", ctx.GetPrefix(),
	)

	bodyText := replacer.Replace(tpl)

	if mediaURL != "" {
		bodyText = bodyText + "\n\n" + mediaURL
	}

	return ctx.Reply(bodyText)
}

func sendAliveCustomizeGuide(ctx *Context) error {
	p := ctx.GetPrefix()
	var sb strings.Builder
	sb.WriteString("╭━━━〔 ALIVE CUSTOMIZATION GUIDE 〕━━━\n\n")
	sb.WriteString("Usage:\n")
	fmt.Fprintf(&sb, "• Check Alive Status : `%salive`\n", p)
	fmt.Fprintf(&sb, "• Custom Message     : `%salive set <your custom template>`\n", p)
	fmt.Fprintf(&sb, "• Custom Media URL   : `%salive media <url | clear>`\n", p)
	fmt.Fprintf(&sb, "• Reset Template     : `%salive set reset`\n\n", p)

	sb.WriteString("Available Placeholders:\n")
	sb.WriteString("- `{bot}`        : Bot display name\n")
	sb.WriteString("- `{owner}`      : Bot owner user ID\n")
	sb.WriteString("- `{uptime}`     : Active system uptime\n")
	sb.WriteString("- `{latency}`    : Response latency\n")
	sb.WriteString("- `{ram}`        : Allocated RAM usage\n")
	sb.WriteString("- `{goroutines}` : Active Go routines\n")
	sb.WriteString("- `{version}`    : Bot engine version\n")
	sb.WriteString("- `{prefix}`     : Active command prefix\n\n")

	sb.WriteString("Example Custom Template:\n")
	fmt.Fprintf(&sb, "`%salive set Hello! {bot} is running smoothly! Uptime: {uptime}, RAM: {ram}`\n", p)

	return ctx.Reply(strings.TrimSpace(sb.String()))
}

func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	days := d / (24 * time.Hour)
	d -= days * 24 * time.Hour
	hours := d / time.Hour
	d -= hours * time.Hour
	minutes := d / time.Minute
	d -= minutes * time.Minute
	seconds := d / time.Second

	parts := []string{}
	if days > 0 {
		parts = append(parts, fmt.Sprintf("%dd", days))
	}
	if hours > 0 {
		parts = append(parts, fmt.Sprintf("%dh", hours))
	}
	if minutes > 0 {
		parts = append(parts, fmt.Sprintf("%dm", minutes))
	}
	if seconds > 0 || len(parts) == 0 {
		parts = append(parts, fmt.Sprintf("%ds", seconds))
	}
	return strings.Join(parts, " ")
}
