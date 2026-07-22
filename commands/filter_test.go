package commands

import (
	"testing"
)

func TestFilterCommandsRegistration(t *testing.T) {
	expectedCmds := []struct {
		name     string
		category string
	}{
		{"addfilter", "filters"},
		{"getfilter", "filters"},
		{"listfilters", "filters"},
		{"delfilter", "filters"},
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
