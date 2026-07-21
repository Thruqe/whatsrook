package font

import (
	"strings"
	"sync"
)

var (
	currentStyle = "monospace"
	mu           sync.RWMutex
)

// SetStyle updates the current global font style.
func SetStyle(style string) {
	mu.Lock()
	defer mu.Unlock()
	currentStyle = strings.ToLower(style)
}

// GetStyle returns the current global font style.
func GetStyle() string {
	mu.RLock()
	defer mu.RUnlock()
	return currentStyle
}

// Convert converts regular alphanumeric characters in a string to the current selected fancy style.
func Convert(s string) string {
	style := GetStyle()
	var sb strings.Builder
	for _, r := range s {
		switch style {
		case "monospace":
			if r >= 'a' && r <= 'z' {
				sb.WriteRune(r - 'a' + 0x1D68A)
			} else if r >= 'A' && r <= 'Z' {
				sb.WriteRune(r - 'A' + 0x1D670)
			} else if r >= '0' && r <= '9' {
				sb.WriteRune(r - '0' + 0x1D7F6)
			} else {
				sb.WriteRune(r)
			}
		case "bold":
			if r >= 'a' && r <= 'z' {
				sb.WriteRune(r - 'a' + 0x1D5BA)
			} else if r >= 'A' && r <= 'Z' {
				sb.WriteRune(r - 'A' + 0x1D5A0)
			} else if r >= '0' && r <= '9' {
				sb.WriteRune(r - '0' + 0x1D7EC)
			} else {
				sb.WriteRune(r)
			}
		case "script":
			if r >= 'a' && r <= 'z' {
				sb.WriteRune(r - 'a' + 0x1D4EA)
			} else if r >= 'A' && r <= 'Z' {
				sb.WriteRune(r - 'A' + 0x1D4D0)
			} else {
				sb.WriteRune(r)
			}
		default:
			sb.WriteRune(r)
		}
	}
	return sb.String()
}
