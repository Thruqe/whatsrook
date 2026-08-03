// Automute & Autounmute group management plugin – schedules automatic group mute/unmute times with timezone support and interactive pagination.
package commands

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"whatsrook/store/sqlstore"
	"whatsrook/utils"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

// Supported timezones list for interactive pagination selection
var supportedTimezones = []string{
	"UTC",
	"Africa/Abidjan",
	"Africa/Accra",
	"Africa/Addis_Ababa",
	"Africa/Algiers",
	"Africa/Cairo",
	"Africa/Casablanca",
	"Africa/Dakar",
	"Africa/Harare",
	"Africa/Johannesburg",
	"Africa/Lagos",
	"Africa/Nairobi",
	"Africa/Tunis",
	"America/Anchorage",
	"America/Argentina/Buenos_Aires",
	"America/Bogota",
	"America/Chicago",
	"America/Denver",
	"America/Los_Angeles",
	"America/Mexico_City",
	"America/New_York",
	"America/Phoenix",
	"America/Santiago",
	"America/Sao_Paulo",
	"America/Toronto",
	"Asia/Baghdad",
	"Asia/Bangkok",
	"Asia/Colombo",
	"Asia/Dhaka",
	"Asia/Dubai",
	"Asia/Hong_Kong",
	"Asia/Jakarta",
	"Asia/Jerusalem",
	"Asia/Karachi",
	"Asia/Kolkata",
	"Asia/Kathmandu",
	"Asia/Kuala_Lumpur",
	"Asia/Manila",
	"Asia/Riyadh",
	"Asia/Seoul",
	"Asia/Shanghai",
	"Asia/Singapore",
	"Asia/Taipei",
	"Asia/Tehran",
	"Asia/Tokyo",
	"Australia/Adelaide",
	"Australia/Brisbane",
	"Australia/Melbourne",
	"Australia/Perth",
	"Australia/Sydney",
	"Europe/Amsterdam",
	"Europe/Athens",
	"Europe/Berlin",
	"Europe/Brussels",
	"Europe/Dublin",
	"Europe/Istanbul",
	"Europe/Lisbon",
	"Europe/London",
	"Europe/Madrid",
	"Europe/Moscow",
	"Europe/Paris",
	"Europe/Rome",
	"Europe/Warsaw",
	"Europe/Zurich",
	"Pacific/Auckland",
	"Pacific/Fiji",
	"Pacific/Honolulu",
}

type MuteSchedule struct {
	GroupJID   string `json:"group_jid"`
	MuteTime   string `json:"mute_time"`   // e.g. "22:00"
	UnmuteTime string `json:"unmute_time"` // e.g. "06:00"
	Enabled    bool   `json:"enabled"`
}

var (
	autoMuteSchedulerOnce sync.Once
)

func init() {
	sort.Strings(supportedTimezones)

	Register(&Command{
		Name:        "automute",
		Aliases:     []string{"setautomute", "autoclose"},
		Description: "Configure automatic daily group mute (close) time in HH:MM (24h format)",
		Category:    "group",
		GroupOnly:   true,
		IsPublic:    true,
		Handler:     handleAutoMute,
	})
	Register(&Command{
		Name:        "autounmute",
		Aliases:     []string{"setautounmute", "autoopen"},
		Description: "Configure automatic daily group unmute (open) time in HH:MM (24h format)",
		Category:    "group",
		GroupOnly:   true,
		IsPublic:    true,
		Handler:     handleAutoUnmute,
	})
	Register(&Command{
		Name:        "listmute",
		Aliases:     []string{"mutestatus", "automutestatus", "muteschedule"},
		Description: "List active automute & autounmute schedules for this group",
		Category:    "group",
		GroupOnly:   true,
		IsPublic:    true,
		Handler:     handleListMute,
	})
	Register(&Command{
		Name:        "timezone",
		Aliases:     []string{"tz", "settimezone", "settz"},
		Description: "View or configure timezone for automute schedules via interactive buttons",
		Category:    "settings",
		IsPublic:    true,
		Handler:     handleTimezone,
	})
}

// StartAutoMuteScheduler initializes the 1-minute ticker to check and trigger automute schedules
func StartAutoMuteScheduler(ctx context.Context, client *whatsmeow.Client) {
	autoMuteSchedulerOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(1 * time.Minute)
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					checkAndExecuteMuteSchedules(ctx, client)
				}
			}
		}()
	})
}

func handleAutoMute(ctx *Context) error {
	if ctx.Chat.Server != types.GroupServer {
		return ctx.Reply("This command can only be used in a group.")
	}
	info, err := ctx.Client.GetGroupInfo(ctx.Ctx, ctx.Chat)
	if err != nil {
		return ctx.Reply(fmt.Sprintf("Failed to get group info: %v", err))
	}
	if !ctx.IsSenderAdmin(info) {
		return ctx.Reply("Only group admins can set automute schedules.")
	}

	p := ctx.GetPrefix()
	if len(ctx.Args) == 0 {
		return ctx.Reply(fmt.Sprintf("Usage:\n- `%sautomute 22:00` (Sets daily automute at 10:00 PM)\n- `%sautomute off` (Disables automute)", p, p))
	}

	arg := strings.ToLower(ctx.Args[0])
	s, ok := ctx.Client.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return ctx.Reply("Settings store unavailable.")
	}

	settingKey := "automute:" + ctx.Chat.String()

	if arg == "off" || arg == "disable" || arg == "del" {
		_ = s.DeleteSetting(ctx.Ctx, settingKey)
		return ctx.Reply("Automute schedule disabled for this group.")
	}

	if !isValidHHMM(arg) {
		return ctx.Reply("Invalid time format. Please specify time in HH:MM (24-hour format), e.g. `22:00` or `23:30`.")
	}

	err = s.PutSetting(ctx.Ctx, settingKey, arg)
	if err != nil {
		return ctx.Reply("Failed to save automute schedule.")
	}

	tz := getUserTimezone(ctx.Ctx, s)
	return ctx.Reply(fmt.Sprintf("Automute schedule set to *%s* daily (Timezone: *%s*).\nThe group will close automatically at %s every day.", arg, tz, arg))
}

func handleAutoUnmute(ctx *Context) error {
	if ctx.Chat.Server != types.GroupServer {
		return ctx.Reply("This command can only be used in a group.")
	}
	info, err := ctx.Client.GetGroupInfo(ctx.Ctx, ctx.Chat)
	if err != nil {
		return ctx.Reply(fmt.Sprintf("Failed to get group info: %v", err))
	}
	if !ctx.IsSenderAdmin(info) {
		return ctx.Reply("Only group admins can set autounmute schedules.")
	}

	p := ctx.GetPrefix()
	if len(ctx.Args) == 0 {
		return ctx.Reply(fmt.Sprintf("Usage:\n- `%sautounmute 06:00` (Sets daily autounmute at 06:00 AM)\n- `%sautounmute off` (Disables autounmute)", p, p))
	}

	arg := strings.ToLower(ctx.Args[0])
	s, ok := ctx.Client.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return ctx.Reply("Settings store unavailable.")
	}

	settingKey := "autounmute:" + ctx.Chat.String()

	if arg == "off" || arg == "disable" || arg == "del" {
		_ = s.DeleteSetting(ctx.Ctx, settingKey)
		return ctx.Reply("Autounmute schedule disabled for this group.")
	}

	if !isValidHHMM(arg) {
		return ctx.Reply("Invalid time format. Please specify time in HH:MM (24-hour format), e.g. `06:00` or `07:30`.")
	}

	err = s.PutSetting(ctx.Ctx, settingKey, arg)
	if err != nil {
		return ctx.Reply("Failed to save autounmute schedule.")
	}

	tz := getUserTimezone(ctx.Ctx, s)
	return ctx.Reply(fmt.Sprintf("Autounmute schedule set to *%s* daily (Timezone: *%s*).\nThe group will open automatically at %s every day.", arg, tz, arg))
}

func handleListMute(ctx *Context) error {
	if ctx.Chat.Server != types.GroupServer {
		return ctx.Reply("This command can only be used in a group.")
	}

	s, ok := ctx.Client.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return ctx.Reply("Settings store unavailable.")
	}

	muteTime, _ := s.GetSetting(ctx.Ctx, "automute:"+ctx.Chat.String())
	unmuteTime, _ := s.GetSetting(ctx.Ctx, "autounmute:"+ctx.Chat.String())
	tz := getUserTimezone(ctx.Ctx, s)

	p := ctx.GetPrefix()
	var sb strings.Builder
	sb.WriteString("*Group Mute/Unmute Schedule Status*\n\n")
	fmt.Fprintf(&sb, "*Configured Timezone:* %s\n\n", tz)

	if muteTime != "" {
		fmt.Fprintf(&sb, "*Automute (Group Close):* %s daily\n", muteTime)
	} else {
		sb.WriteString("*Automute (Group Close):* Disabled\n")
	}

	if unmuteTime != "" {
		fmt.Fprintf(&sb, "*Autounmute (Group Open):* %s daily\n", unmuteTime)
	} else {
		sb.WriteString("*Autounmute (Group Open):* Disabled\n")
	}

	sb.WriteString("\nCommands:\n")
	fmt.Fprintf(&sb, "- `%sautomute <HH:MM>` (e.g. `%sautomute 22:00`)\n", p, p)
	fmt.Fprintf(&sb, "- `%sautounmute <HH:MM>` (e.g. `%sautounmute 06:00`)\n", p, p)
	fmt.Fprintf(&sb, "- `%stimezone` (to configure bot timezone)", p)

	return ctx.Reply(sb.String())
}

func handleTimezone(ctx *Context) error {
	s, ok := ctx.Client.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return ctx.Reply("Settings store unavailable.")
	}

	if len(ctx.Args) >= 2 && strings.ToLower(ctx.Args[0]) == "set" {
		tzName := ctx.Args[1]
		if decoded, err := url.QueryUnescape(tzName); err == nil {
			tzName = decoded
		}

		// Try direct IANA load first
		if _, err := time.LoadLocation(tzName); err != nil {
			// Fall back to Windows-name/abbreviation alias resolution
			if resolved, ok := utils.ResolveTimezoneAlias(tzName); ok {
				tzName = resolved
			} else {
				return ctx.Reply(fmt.Sprintf("Invalid timezone: %q. Please select a valid IANA timezone, Windows timezone name, or abbreviation.", tzName))
			}
		}

		err := s.PutSetting(ctx.Ctx, "timezone", tzName)
		if err != nil {
			return ctx.Reply("Failed to save timezone setting.")
		}
		return ctx.Reply(fmt.Sprintf("Bot timezone successfully set to *%s*.", tzName))
	}

	if len(ctx.Args) >= 2 && strings.ToLower(ctx.Args[0]) == "page" {
		pageNum, _ := strconv.Atoi(ctx.Args[1])
		return renderTimezonePage(ctx, s, pageNum)
	}

	if len(ctx.Args) == 1 {
		tzName := ctx.Args[0]
		if _, err := time.LoadLocation(tzName); err == nil {
			_ = s.PutSetting(ctx.Ctx, "timezone", tzName)
			return ctx.Reply(fmt.Sprintf("Bot timezone successfully set to *%s*.", tzName))
		}
	}

	return renderTimezonePage(ctx, s, 1)
}

func renderTimezonePage(ctx *Context, s *sqlstore.SQLStore, page int) error {
	currentTZ := getUserTimezone(ctx.Ctx, s)

	pageSize := 3
	totalPages := (len(supportedTimezones) + pageSize - 1) / pageSize
	if page < 1 {
		page = 1
	}
	if page > totalPages {
		page = totalPages
	}

	startIdx := (page - 1) * pageSize
	endIdx := startIdx + pageSize
	if endIdx > len(supportedTimezones) {
		endIdx = len(supportedTimezones)
	}

	pageItems := supportedTimezones[startIdx:endIdx]
	p := ctx.GetPrefix()

	var sb strings.Builder
	fmt.Fprintf(&sb, "*Timezone Configuration* (Page %d of %d, Total: %d)\n\n", page, totalPages, len(supportedTimezones))
	fmt.Fprintf(&sb, "*Current Timezone:* %s\n\n", currentTZ)
	sb.WriteString("Select your local timezone below so automute & autounmute execute at your exact local time:\n\n")

	for idx, tz := range pageItems {
		globalIdx := startIdx + idx + 1
		loc, err := time.LoadLocation(tz)
		offsetStr := ""
		if err == nil {
			now := time.Now().In(loc)
			_, offset := now.Zone()
			hours := offset / 3600
			mins := (offset % 3600) / 60
			if mins < 0 {
				mins = -mins
			}
			offsetStr = fmt.Sprintf(" (UTC%+03d:%02d)", hours, mins)
		}
		fmt.Fprintf(&sb, "%d. *%s*%s\n", globalIdx, tz, offsetStr)
	}

	var buttons []struct{ ID, Text string }
	for _, tz := range pageItems {
		btnText := tz
		if len(btnText) > 20 {
			btnText = btnText[:20]
		}
		buttons = append(buttons, struct{ ID, Text string }{
			ID:   fmt.Sprintf("%stimezone set %s", p, tz),
			Text: btnText,
		})
	}

	if page < totalPages {
		nextPage := page + 1
		buttons = append(buttons, struct{ ID, Text string }{
			ID:   fmt.Sprintf("%stimezone page %d", p, nextPage),
			Text: fmt.Sprintf("Next (Page %d)", nextPage),
		})
	} else if page > 1 {
		buttons = append(buttons, struct{ ID, Text string }{
			ID:   fmt.Sprintf("%stimezone page 1", p),
			Text: "First Page",
		})
	}

	sb.WriteString("\nTap a button above to select your timezone, or type:\n")
	fmt.Fprintf(&sb, "`%stimezone <Name>` (e.g. `%stimezone Africa/Lagos`)", p, p)

	return sendInteractiveButtons(ctx, sb.String(), fmt.Sprintf("Powered by %s", ctx.GetBotName()), buttons)
}

func getUserTimezone(ctx context.Context, s *sqlstore.SQLStore) string {
	tz, err := s.GetSetting(ctx, "timezone")
	if err != nil || tz == "" {
		return "UTC"
	}
	return tz
}

func isValidHHMM(s string) bool {
	if len(s) != 5 || s[2] != ':' {
		return false
	}
	hours, err1 := strconv.Atoi(s[:2])
	mins, err2 := strconv.Atoi(s[3:])
	if err1 != nil || err2 != nil {
		return false
	}
	return hours >= 0 && hours <= 23 && mins >= 0 && mins <= 59
}

func checkAndExecuteMuteSchedules(ctx context.Context, client *whatsmeow.Client) {
	if client == nil || client.Store == nil {
		return
	}
	s, ok := client.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return
	}

	db := s.GetDB()
	if db == nil {
		return
	}

	tzName := getUserTimezone(ctx, s)
	loc, err := time.LoadLocation(tzName)
	if err != nil {
		loc = time.UTC
	}

	now := time.Now().In(loc)
	currentTimeStr := fmt.Sprintf("%02d:%02d", now.Hour(), now.Minute())

	rows, err := db.Query(ctx, `SELECT key, value FROM bot_settings WHERE our_jid=$1 AND (key LIKE 'automute:%' OR key LIKE 'autounmute:%')`, s.JID)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var key, targetTime string
		if err := rows.Scan(&key, &targetTime); err != nil {
			continue
		}

		if targetTime != currentTimeStr {
			continue
		}

		if strings.HasPrefix(key, "automute:") {
			groupJIDStr := strings.TrimPrefix(key, "automute:")
			groupJID, err := types.ParseJID(groupJIDStr)
			if err != nil || groupJID.Server != types.GroupServer {
				continue
			}

			// Check last execution to avoid duplicate trigger within same minute
			execKey := "last_exec_automute:" + groupJIDStr
			lastExec, _ := s.GetSetting(ctx, execKey)
			dateMinuteKey := fmt.Sprintf("%s_%s", now.Format("2006-01-02"), currentTimeStr)
			if lastExec == dateMinuteKey {
				continue
			}

			info, gErr := client.GetGroupInfo(ctx, groupJID)
			if gErr == nil && info != nil {
				// Check if bot is admin
				botJID := client.Store.ID.ToNonAD()
				isAdmin := false
				for _, p := range info.Participants {
					if p.JID.ToNonAD() == botJID && (p.IsAdmin || p.IsSuperAdmin) {
						isAdmin = true
						break
					}
				}
				if isAdmin {
					_ = client.SetGroupAnnounce(ctx, groupJID, true)
					_ = s.PutSetting(ctx, execKey, dateMinuteKey)
					slog.Info("Automute executed for group", "group", groupJIDStr, "time", currentTimeStr)
				}
			}
		} else if strings.HasPrefix(key, "autounmute:") {
			groupJIDStr := strings.TrimPrefix(key, "autounmute:")
			groupJID, err := types.ParseJID(groupJIDStr)
			if err != nil || groupJID.Server != types.GroupServer {
				continue
			}

			execKey := "last_exec_autounmute:" + groupJIDStr
			lastExec, _ := s.GetSetting(ctx, execKey)
			dateMinuteKey := fmt.Sprintf("%s_%s", now.Format("2006-01-02"), currentTimeStr)
			if lastExec == dateMinuteKey {
				continue
			}

			info, gErr := client.GetGroupInfo(ctx, groupJID)
			if gErr == nil && info != nil {
				botJID := client.Store.ID.ToNonAD()
				isAdmin := false
				for _, p := range info.Participants {
					if p.JID.ToNonAD() == botJID && (p.IsAdmin || p.IsSuperAdmin) {
						isAdmin = true
						break
					}
				}
				if isAdmin {
					_ = client.SetGroupAnnounce(ctx, groupJID, false)
					_ = s.PutSetting(ctx, execKey, dateMinuteKey)
					slog.Info("Autounmute executed for group", "group", groupJIDStr, "time", currentTimeStr)
				}
			}
		}
	}
}
