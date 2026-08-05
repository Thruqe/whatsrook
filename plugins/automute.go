// Automute & Autounmute group management plugin – schedules automatic group mute/unmute times with timezone support and interactive pagination.
package plugins

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

	"go.mau.fi/whatsmeow/proto/waE2E"

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

// StartAutoMuteScheduler initializes a 1-second ticker to check and trigger
// automute schedules. Checking every second (instead of drifting on a 1-minute
// ticker) guarantees we never miss the exact HH:MM boundary due to tick drift.
func StartAutoMuteScheduler(ctx context.Context, client *whatsmeow.Client) {
	autoMuteSchedulerOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(1 * time.Second)
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					func() {
						defer func() {
							if r := recover(); r != nil {
								slog.Error("automute: PANIC in scheduler tick", "recover", r)
							}
						}()
						checkAndExecuteMuteSchedules(ctx, client)
					}()
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

	normalized, ok := normalizeTimeInput(arg)
	if !ok {
		return ctx.Reply("Invalid time format. Please specify time as HH:MM (24h, e.g. `22:00`) or H:MM AM/PM (12h, e.g. `10:00 PM`).")
	}
	arg = normalized

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

	normalized, ok := normalizeTimeInput(arg)
	if !ok {
		return ctx.Reply("Invalid time format. Please specify time as HH:MM (24h, e.g. `22:00`) or H:MM AM/PM (12h, e.g. `10:00 PM`).")
	}
	arg = normalized

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

	if len(ctx.Args) >= 2 && strings.ToLower(ctx.Args[0]) == "setidx" {
		idx, err := strconv.Atoi(ctx.Args[1])
		if err != nil || idx < 1 || idx > len(supportedTimezones) {
			return ctx.Reply("Invalid timezone selection.")
		}
		tzName := supportedTimezones[idx-1]
		if err := s.PutSetting(ctx.Ctx, "timezone", tzName); err != nil {
			return ctx.Reply("Failed to save timezone setting.")
		}
		return ctx.Reply(fmt.Sprintf("Bot timezone successfully set to *%s*.", tzName))
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
	for idx, tz := range pageItems {
		globalIdx := startIdx + idx + 1
		btnText := tz
		if len(btnText) > 20 {
			btnText = btnText[:20]
		}
		buttons = append(buttons, struct{ ID, Text string }{
			ID:   fmt.Sprintf("%stimezone setidx %d", p, globalIdx),
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

// normalizeTimeInput accepts "HH:MM" (24h) or "H:MM AM/PM" / "HH:MM AM/PM" (12h,
// case-insensitive, with or without a space before AM/PM) and returns the
// canonical 24-hour "HH:MM" string. Returns ("", false) if invalid.
func normalizeTimeInput(s string) (string, bool) {
	s = strings.TrimSpace(s)
	upper := strings.ToUpper(s)

	// 12-hour format: optional space before AM/PM, e.g. "10:00PM", "10:00 PM", "6:30am"
	if strings.HasSuffix(upper, "AM") || strings.HasSuffix(upper, "PM") {
		isPM := strings.HasSuffix(upper, "PM")
		timePart := strings.TrimSpace(strings.TrimSuffix(strings.TrimSuffix(upper, "AM"), "PM"))

		parts := strings.Split(timePart, ":")
		if len(parts) != 2 {
			return "", false
		}
		hour, err1 := strconv.Atoi(parts[0])
		minute, err2 := strconv.Atoi(parts[1])
		if err1 != nil || err2 != nil {
			return "", false
		}
		if hour < 1 || hour > 12 || minute < 0 || minute > 59 {
			return "", false
		}

		// Convert 12h -> 24h
		if isPM && hour != 12 {
			hour += 12
		}
		if !isPM && hour == 12 {
			hour = 0
		}
		return fmt.Sprintf("%02d:%02d", hour, minute), true
	}

	// 24-hour format: strict "HH:MM"
	if len(s) != 5 || s[2] != ':' {
		return "", false
	}
	hours, err1 := strconv.Atoi(s[:2])
	mins, err2 := strconv.Atoi(s[3:])
	if err1 != nil || err2 != nil {
		return "", false
	}
	if hours < 0 || hours > 23 || mins < 0 || mins > 59 {
		return "", false
	}
	return s, true
}

func checkAndExecuteMuteSchedules(ctx context.Context, client *whatsmeow.Client) {
	if client == nil || client.Store == nil {
		slog.Warn("automute: client or store is nil, skipping tick")
		return
	}
	s, ok := client.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		slog.Warn("automute: Identities is not *sqlstore.SQLStore, skipping tick")
		return
	}

	db := s.GetDB()
	if db == nil {
		slog.Warn("automute: GetDB() returned nil, skipping tick")
		return
	}

	tzName := getUserTimezone(ctx, s)
	loc, err := time.LoadLocation(tzName)
	if err != nil {
		slog.Warn("automute: failed to load timezone, falling back to UTC", "tz", tzName, "err", err)
		loc = time.UTC
	}

	now := time.Now().In(loc)
	currentTimeStr := fmt.Sprintf("%02d:%02d", now.Hour(), now.Minute())

	rows, err := db.Query(ctx, `SELECT key, value FROM bot_settings WHERE our_jid=$1 AND (key LIKE 'automute:%' OR key LIKE 'autounmute:%')`, s.JID)
	if err != nil {
		slog.Error("automute: query failed", "err", err)
		return
	}
	defer rows.Close()

	rowCount := 0
	for rows.Next() {
		rowCount++
		var key, targetTime string
		if err := rows.Scan(&key, &targetTime); err != nil {
			slog.Error("automute: row scan failed", "err", err)
			continue
		}

		if targetTime != currentTimeStr {
			continue
		}

		if strings.HasPrefix(key, "automute:") {
			groupJIDStr := strings.TrimPrefix(key, "automute:")
			slog.Debug("automute: match found, entering automute branch", "group_raw", groupJIDStr)
			groupJID, err := types.ParseJID(groupJIDStr)
			if err != nil || groupJID.Server != types.GroupServer {
				slog.Warn("automute: bad group JID, skipping", "raw", groupJIDStr, "err", err)
				continue
			}
			slog.Debug("automute: JID parsed ok", "group", groupJID.String())

			execKey := "last_exec_automute:" + groupJIDStr
			sCtx, sCancel := context.WithTimeout(ctx, 5*time.Second)
			lastExec, sErr := s.GetSetting(sCtx, execKey)
			sCancel()
			if sErr != nil {
				slog.Error("automute: GetSetting execKey failed or timed out", "group", groupJIDStr, "err", sErr)
				continue
			}
			dateMinuteKey := fmt.Sprintf("%s_%s", now.Format("2006-01-02"), currentTimeStr)
			if lastExec == dateMinuteKey {
				slog.Debug("automute: already executed this minute, skipping", "group", groupJIDStr, "key", dateMinuteKey)
				continue
			}

			info, gErr := client.GetGroupInfo(ctx, groupJID)
			if gErr != nil {
				slog.Error("automute: GetGroupInfo failed", "group", groupJIDStr, "err", gErr)
				continue
			}
			if info == nil {
				slog.Warn("automute: GetGroupInfo returned nil info", "group", groupJIDStr)
				continue
			}

			botJID := client.Store.ID.ToNonAD()
			botLID := client.Store.GetLID().ToNonAD()
			isAdmin := false
			for _, p := range info.Participants {
				matchesBot := (p.PhoneNumber.IsEmpty() == false && p.PhoneNumber.ToNonAD() == botJID) ||
					(p.LID.IsEmpty() == false && p.LID.ToNonAD() == botLID) ||
					(p.JID.ToNonAD() == botJID)
				if matchesBot && (p.IsAdmin || p.IsSuperAdmin) {
					isAdmin = true
					break
				}
			}
			slog.Debug("automute: admin check", "group", groupJIDStr, "bot_jid", botJID.String(), "is_admin", isAdmin)

			if !isAdmin {
				slog.Warn("automute: bot is not admin in group, cannot mute", "group", groupJIDStr)
				continue
			}

			if err := client.SetGroupAnnounce(ctx, groupJID, true); err != nil {
				slog.Error("automute: SetGroupAnnounce(true) failed", "group", groupJIDStr, "err", err)
				continue
			}
			if err := s.PutSetting(ctx, execKey, dateMinuteKey); err != nil {
				slog.Error("automute: failed to save last_exec marker", "group", groupJIDStr, "err", err)
			}
			slog.Info("automute: executed successfully", "group", groupJIDStr, "time", currentTimeStr)

			unmuteTime, _ := s.GetSetting(ctx, "autounmute:"+groupJIDStr)
			groupName := info.GroupName.Name
			var noticeText string
			if unmuteTime != "" {
				noticeText = fmt.Sprintf("*%s* has been closed, and will be opened by *%s* at *%s*.", groupName, unmuteTime, tzName)
			} else {
				noticeText = fmt.Sprintf("*%s* has been closed.", groupName)
			}
			if _, sendErr := client.SendMessage(ctx, groupJID, &waE2E.Message{Conversation: &noticeText}); sendErr != nil {
				slog.Error("automute: failed to send close notice", "group", groupJIDStr, "err", sendErr)
			}

		} else if strings.HasPrefix(key, "autounmute:") {
			groupJIDStr := strings.TrimPrefix(key, "autounmute:")
			groupJID, err := types.ParseJID(groupJIDStr)
			if err != nil || groupJID.Server != types.GroupServer {
				slog.Warn("autounmute: bad group JID, skipping", "raw", groupJIDStr, "err", err)
				continue
			}

			execKey := "last_exec_autounmute:" + groupJIDStr
			lastExec, _ := s.GetSetting(ctx, execKey)
			dateMinuteKey := fmt.Sprintf("%s_%s", now.Format("2006-01-02"), currentTimeStr)
			if lastExec == dateMinuteKey {
				slog.Debug("autounmute: already executed this minute, skipping", "group", groupJIDStr, "key", dateMinuteKey)
				continue
			}

			info, gErr := client.GetGroupInfo(ctx, groupJID)
			if gErr != nil {
				slog.Error("autounmute: GetGroupInfo failed", "group", groupJIDStr, "err", gErr)
				continue
			}
			if info == nil {
				slog.Warn("autounmute: GetGroupInfo returned nil info", "group", groupJIDStr)
				continue
			}

			botJID := client.Store.ID.ToNonAD()
			botLID := client.Store.GetLID().ToNonAD()
			isAdmin := false
			for _, p := range info.Participants {
				matchesBot := (p.PhoneNumber.IsEmpty() == false && p.PhoneNumber.ToNonAD() == botJID) ||
					(p.LID.IsEmpty() == false && p.LID.ToNonAD() == botLID) ||
					(p.JID.ToNonAD() == botJID) // fallback for older/PN-addressed groups
				if matchesBot && (p.IsAdmin || p.IsSuperAdmin) {
					isAdmin = true
					break
				}
			}
			slog.Debug("autounmute: admin check", "group", groupJIDStr, "bot_jid", botJID.String(), "is_admin", isAdmin)

			if !isAdmin {
				slog.Warn("autounmute: bot is not admin in group, cannot unmute", "group", groupJIDStr)
				continue
			}

			if err := client.SetGroupAnnounce(ctx, groupJID, false); err != nil {
				slog.Error("autounmute: SetGroupAnnounce(false) failed", "group", groupJIDStr, "err", err)
				continue
			}
			if err := s.PutSetting(ctx, execKey, dateMinuteKey); err != nil {
				slog.Error("autounmute: failed to save last_exec marker", "group", groupJIDStr, "err", err)
			}

			slog.Info("autounmute: executed successfully", "group", groupJIDStr, "time", currentTimeStr)
			groupName := info.GroupName.Name
			noticeText := fmt.Sprintf("*%s* has been opened.", groupName)
			if _, sendErr := client.SendMessage(ctx, groupJID, &waE2E.Message{Conversation: &noticeText}); sendErr != nil {
				slog.Error("autounmute: failed to send open notice", "group", groupJIDStr, "err", sendErr)
			}
		}
	}
}
