package commands

import (
	"testing"
)

func TestAntiCallCommandRegistration(t *testing.T) {
	cmd, ok := Get("anticall")
	if !ok {
		t.Fatal("expected 'anticall' command to be registered")
	}

	if cmd.Category != "calls" {
		t.Errorf("expected category 'calls', got %q", cmd.Category)
	}

	if cmd.Handler == nil {
		t.Error("expected command handler to be set, got nil")
	}

	if cmd.IsPublic {
		t.Error("expected anticall command to be restricted (IsPublic: false)")
	}
}
