package commands

import (
	"fmt"
	"math"
	"os"
	"runtime"
	"strings"
	"time"
)

func init() {
	Register(&Command{
		Name:        "menu",
		Description: "Show all available commands grouped by category",
		Category:    "info",
		Handler:     handleMenu,
	})
}

func handleMenu(ctx *Context) error {
	type entry struct{ name, desc string }
	categoryOrder := []string{}
	categories := map[string][]entry{}
	seenCat := map[string]bool{}

	for _, cmd := range Visible() {
		cat := cmd.Category
		if cat == "" {
			cat = "misc"
		}
		if !seenCat[cat] {
			seenCat[cat] = true
			categoryOrder = append(categoryOrder, cat)
		}
		categories[cat] = append(categories[cat], entry{name: cmd.Name, desc: cmd.Description})
	}

	uptime := menuRuntime(time.Since(startTime).Seconds())
	totalRAM, freeRAM := memStats()
	usedRAM := totalRAM - freeRAM
	platform := runtime.GOOS
	total := len(Visible())

	user := ctx.Evt.Info.PushName
	if user == "" {
		user = ctx.Sender.User
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "╭━━━〔 *WhatsRook* 〕━━━\n")
	fmt.Fprintf(&sb, "│╭──────────────\n")
	fmt.Fprintf(&sb, "││ User    : %s\n", user)
	fmt.Fprintf(&sb, "││ Plugins : %d\n", total)
	fmt.Fprintf(&sb, "││ Runtime : %s\n", uptime)
	fmt.Fprintf(&sb, "││ Platform: %s\n", platform)
	fmt.Fprintf(&sb, "││ RAM     : %s / %s\n", formatBytes(usedRAM), formatBytes(totalRAM))
	fmt.Fprintf(&sb, "│╰──────────────\n")
	fmt.Fprintf(&sb, "╰━━━━━━━━━━━━━━━\n\n")

	for _, cat := range categoryOrder {
		cmds := categories[cat]
		catLabel := "*〔 " + strings.ToUpper(cat) + " 〕*"

		fmt.Fprintf(&sb, "╭─────────────\n")
		fmt.Fprintf(&sb, "│ %s\n", catLabel)
		fmt.Fprintf(&sb, "╰┬────────────\n")
		fmt.Fprintf(&sb, "┌┤\n")

		for _, e := range cmds {
			fmt.Fprintf(&sb, "││◦ %s\n", e.name)
		}

		fmt.Fprintf(&sb, "│╰────────────\n")
		fmt.Fprintf(&sb, "╰─────────────\n\n")
	}

	return sendText(ctx, strings.TrimRight(sb.String(), "\n"))
}

// menuRuntime formats a duration in seconds as "Xd Xh Xm Xs".
func menuRuntime(seconds float64) string {
	secs := int(math.Floor(seconds))
	d := secs / (3600 * 24)
	h := (secs % (3600 * 24)) / 3600
	m := (secs % 3600) / 60
	s := secs % 60

	var parts []string
	if d > 0 {
		parts = append(parts, fmt.Sprintf("%dd", d))
	}
	if h > 0 {
		parts = append(parts, fmt.Sprintf("%dh", h))
	}
	if m > 0 {
		parts = append(parts, fmt.Sprintf("%dm", m))
	}
	if s > 0 || len(parts) == 0 {
		parts = append(parts, fmt.Sprintf("%ds", s))
	}
	return strings.Join(parts, " ")
}

// formatBytes formats a byte count into a human-readable string (KB/MB/GB).
func formatBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// memStats returns (totalRAM, freeRAM) in bytes by reading /proc/meminfo on
// Linux. Falls back to runtime.MemStats (heap only) on other platforms.
func memStats() (total, free uint64) {
	if runtime.GOOS == "linux" {
		if t, f, err := parseProcMeminfo(); err == nil {
			return t, f
		}
	}
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return ms.Sys, ms.Sys - ms.HeapInuse
}

func parseProcMeminfo() (total, free uint64, err error) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, 0, err
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		var key string
		var val uint64
		if _, scanErr := fmt.Sscanf(line, "%s %d", &key, &val); scanErr != nil {
			continue
		}
		val *= 1024 // /proc/meminfo is in kB
		switch key {
		case "MemTotal:":
			total = val
		case "MemAvailable:":
			free = val
		}
	}
	return total, free, nil
}
