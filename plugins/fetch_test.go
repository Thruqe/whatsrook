package commands

import (
	"testing"
)

func TestFetchAndDlCommandsRegistration(t *testing.T) {
	expectedCmds := []struct {
		name     string
		category string
	}{
		{"dl", "downloader"},
		{"fetch", "downloader"},
	}

	for _, tc := range expectedCmds {
		cmd, ok := Get(tc.name)
		if !ok {
			t.Errorf("expected command %s to be registered", tc.name)
			continue
		}
		if cmd.Category != tc.category {
			t.Errorf("expected command %s category to be %s, got %s", tc.name, tc.category, cmd.Category)
		}
		if cmd.Handler == nil {
			t.Errorf("expected command %s handler to not be nil", tc.name)
		}
	}
}

func TestIsValidHeaderKey(t *testing.T) {
	tests := []struct {
		key   string
		valid bool
	}{
		{"Content-Type", true},
		{"User-Agent", true},
		{"X-Custom-123", true},
		{"Content_Type", false},
		{"Content:Type", false},
		{"", false},
		{"Accept", true},
	}

	for _, tc := range tests {
		got := isValidHeaderKey(tc.key)
		if got != tc.valid {
			t.Errorf("isValidHeaderKey(%q) = %v; want %v", tc.key, got, tc.valid)
		}
	}
}
