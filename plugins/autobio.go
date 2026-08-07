// AutoBio command – automatically updates the bot's WhatsApp status bio every minute with current time and inspirational quotes.
package plugins

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"strings"
	"sync"
	"time"

	"whatsrook/store/sqlstore"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

var (
	autoBioOnce     sync.Once
	autoBioRng      = rand.New(rand.NewSource(time.Now().UnixNano()))
	autoBioRngMutex sync.Mutex
)

var bioQuotes = []string{
	"Believe you can and you're halfway there.",
	"The only way to do great work is to love what you do.",
	"Act as if what you do makes a difference. It does.",
	"Dream big and dare to fail.",
	"What we think, we become.",
	"Stay hungry, stay foolish.",
	"Turn your wounds into wisdom.",
	"Do what you can, with what you have, where you are.",
	"Happiness depends upon ourselves.",
	"Keep your face always toward the sunshine.",
	"Mastering yourself is true power.",
	"Simplicity is the ultimate sophistication.",
	"In the middle of difficulty lies opportunity.",
	"Be yourself; everyone else is already taken.",
	"Strive not to be a success, but to be of value.",
	"Whatever you are, be a good one.",
	"Make each day your masterpiece.",
	"Impossible is for the unwilling.",
	"Where there is love there is life.",
	"Action is the foundational key to all success.",
}

func init() {
	Register(&Command{
		Name:        "autobio",
		Aliases:     []string{"bioauto", "setautobio", "bioupdate"},
		Description: "Auto-update WhatsApp status bio every minute with time & inspirational quotes",
		Category:    "settings",
		IsPublic:    true,
		Handler:     handleAutoBio,
	})
}

// StartAutoBioScheduler initializes the 1-minute ticker for updating WhatsApp status/bio
func StartAutoBioScheduler(ctx context.Context, client *whatsmeow.Client) {
	autoBioOnce.Do(func() {
		go func() {
			time.Sleep(5 * time.Second)
			_, _ = updateAutoBio(ctx, client)

			ticker := time.NewTicker(1 * time.Minute)
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					_, _ = updateAutoBio(ctx, client)
				}
			}
		}()
	})
}

func handleAutoBio(ctx *Context) error {
	s, ok := ctx.Client.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return ctx.Reply("Settings store unavailable.")
	}

	arg0 := ""
	if len(ctx.Args) > 0 {
		arg0 = strings.ToLower(ctx.Args[0])
	}

	switch arg0 {
	case "on", "enable", "true", "start":
		if err := s.PutSetting(ctx.Ctx, "autobio_enabled", "true"); err != nil {
			return ctx.Reply("Failed to enable AutoBio.")
		}
		_, _ = updateAutoBio(ctx.Ctx, ctx.Client)
		return ctx.Reply("AutoBio ENABLED! Status bio will update every minute with local time and quotes.")

	case "off", "disable", "false", "stop":
		if err := s.PutSetting(ctx.Ctx, "autobio_enabled", "false"); err != nil {
			return ctx.Reply("Failed to disable AutoBio.")
		}
		return ctx.Reply("AutoBio DISABLED.")

	case "toggle":
		enabled, _ := s.GetSetting(ctx.Ctx, "autobio_enabled")
		if enabled == "true" {
			_ = s.PutSetting(ctx.Ctx, "autobio_enabled", "false")
			return ctx.Reply("AutoBio DISABLED.")
		}
		_ = s.PutSetting(ctx.Ctx, "autobio_enabled", "true")
		_, _ = updateAutoBio(ctx.Ctx, ctx.Client)
		return ctx.Reply("AutoBio ENABLED!")

	case "customize", "custom", "help":
		return sendAutoBioCustomizeGuide(ctx, s)

	case "tz", "timezone":
		p := ctx.GetPrefix()
		if len(ctx.Args) < 2 {
			tz := getAutoBioTimezone(ctx.Ctx, s)
			return ctx.Reply(fmt.Sprintf("Current AutoBio timezone: %s\n\nTo change timezone:\n- %sautobio tz Africa/Lagos\n- %sautobio tz America/New_York\n- %sautobio tz UTC", tz, p, p, p))
		}
		newTZ := ctx.Args[1]
		if _, err := time.LoadLocation(newTZ); err != nil {
			return ctx.Reply(fmt.Sprintf("Invalid timezone: %q. Please use valid IANA format (e.g. Africa/Lagos, UTC, America/New_York).", newTZ))
		}
		if err := s.PutSetting(ctx.Ctx, "autobio_timezone", newTZ); err != nil {
			return ctx.Reply("Failed to save timezone setting.")
		}
		_, _ = updateAutoBio(ctx.Ctx, ctx.Client)
		return ctx.Reply(fmt.Sprintf("AutoBio timezone updated to: %s!", newTZ))

	case "now", "update":
		bioText, err := updateAutoBio(ctx.Ctx, ctx.Client)
		if err != nil {
			return ctx.Reply(fmt.Sprintf("Failed to update status bio: %v", err))
		}
		return ctx.Reply(fmt.Sprintf("Status bio updated!\n\nNew Bio:\n\"%s\"", bioText))

	case "status":
		enabled, _ := s.GetSetting(ctx.Ctx, "autobio_enabled")
		statusStr := "DISABLED"
		if enabled == "true" {
			statusStr = "ENABLED"
		}
		tzStr := getAutoBioTimezone(ctx.Ctx, s)
		previewBio := generateBioText(tzStr)
		return ctx.Reply(fmt.Sprintf("AutoBio Status: %s\nTimezone: %s\n\nLive Preview:\n\"%s\"", statusStr, tzStr, previewBio))
	}

	return sendAutoBioMenu(ctx, s)
}

func sendAutoBioMenu(ctx *Context, s *sqlstore.SQLStore) error {
	enabled, _ := s.GetSetting(ctx.Ctx, "autobio_enabled")
	statusStr := "DISABLED"
	if enabled == "true" {
		statusStr = "ENABLED"
	}
	tzStr := getAutoBioTimezone(ctx.Ctx, s)

	p := ctx.GetPrefix()
	bodyText := fmt.Sprintf("╭━━━〔 AUTOBIO CONFIGURATION 〕━━━\n│ Status   : %s\n│ Timezone : %s\n╰━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\nChoose an option below to change status or view customization options.", statusStr, tzStr)

	var actionButton struct{ ID, Text string }
	if enabled == "true" {
		actionButton = struct{ ID, Text string }{ID: p + "autobio off", Text: "Deactivate"}
	} else {
		actionButton = struct{ ID, Text string }{ID: p + "autobio on", Text: "Activate"}
	}

	buttons := []struct{ ID, Text string }{
		actionButton,
		{ID: p + "autobio customize", Text: "Customize"},
	}

	return sendInteractiveButtons(ctx, bodyText, fmt.Sprintf("%s AutoBio Updater", ctx.GetBotName()), buttons)
}

func sendAutoBioCustomizeGuide(ctx *Context, s *sqlstore.SQLStore) error {
	p := ctx.GetPrefix()
	tzStr := getAutoBioTimezone(ctx.Ctx, s)
	previewBio := generateBioText(tzStr)

	var sb strings.Builder
	sb.WriteString("╭━━━〔 AUTOBIO CUSTOMIZATION GUIDE 〕━━━\n\n")
	sb.WriteString("Available Customizations:\n")
	fmt.Fprintf(&sb, "• Set Timezone : `%sautobio tz <IANA Timezone>`\n", p)
	fmt.Fprintf(&sb, "• Force Update : `%sautobio now`\n\n", p)

	sb.WriteString("Examples:\n")
	fmt.Fprintf(&sb, "1. `%sautobio tz Africa/Lagos`\n", p)
	fmt.Fprintf(&sb, "2. `%sautobio tz America/New_York`\n", p)
	fmt.Fprintf(&sb, "3. `%sautobio now` (Force status bio refresh right now)\n\n", p)

	fmt.Fprintf(&sb, "Current Live Bio Preview:\n\"%s\"", previewBio)

	return ctx.Reply(strings.TrimSpace(sb.String()))
}

func getAutoBioTimezone(ctx context.Context, s *sqlstore.SQLStore) string {
	tz, err := s.GetSetting(ctx, "autobio_timezone")
	if err == nil && tz != "" {
		return tz
	}
	tzGen, errGen := s.GetSetting(ctx, "timezone")
	if errGen == nil && tzGen != "" {
		return tzGen
	}
	return "UTC"
}

func generateBioText(tzStr string) string {
	loc, err := time.LoadLocation(tzStr)
	if err != nil {
		loc = time.UTC
	}

	now := time.Now().In(loc)
	timeFormatted := now.Format("03:04 AM")

	autoBioRngMutex.Lock()
	quote := bioQuotes[autoBioRng.Intn(len(bioQuotes))]
	autoBioRngMutex.Unlock()

	return fmt.Sprintf("⏰ %s | %s", timeFormatted, quote)
}

func updateAutoBio(ctx context.Context, client *whatsmeow.Client) (string, error) {
	if client == nil || client.Store == nil || client.Store.Identities == nil {
		return "", fmt.Errorf("client store unavailable")
	}

	s, ok := client.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return "", fmt.Errorf("sql store unavailable")
	}

	enabled, err := s.GetSetting(ctx, "autobio_enabled")
	if err != nil || enabled != "true" {
		return "", nil
	}

	tzStr := getAutoBioTimezone(ctx, s)
	bioText := generateBioText(tzStr)

	err = client.SetStatusMessage(ctx, types.SetStatusInput{Text: &bioText})
	if err != nil {
		slog.Error("[AutoBio] Failed to update WhatsApp status message", "err", err)
		return "", err
	}

	slog.Debug("[AutoBio] Updated WhatsApp status bio", "bio", bioText, "timezone", tzStr)
	return bioText, nil
}
