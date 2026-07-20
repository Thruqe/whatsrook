package commands

import (
	"testing"
)

func TestButtonsCommandRegistration(t *testing.T) {
	cmd, ok := Get("buttons")
	if !ok {
		t.Fatal("expected 'buttons' command to be registered")
	}

	if cmd.Category != "misc" {
		t.Errorf("expected category 'misc', got %q", cmd.Category)
	}

	if cmd.Handler == nil {
		t.Error("expected command handler to be set, got nil")
	}

	if !cmd.IsPublic {
		t.Error("expected command to be public")
	}
}
