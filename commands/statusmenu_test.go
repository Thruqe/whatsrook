package commands

import (
	"testing"
)

func TestStatusMenuCommandRegistration(t *testing.T) {
	cmd, ok := Get("statusmenu")
	if !ok {
		t.Fatal("expected 'statusmenu' command to be registered")
	}

	if cmd.Category != "interactive" {
		t.Errorf("expected category 'interactive', got %q", cmd.Category)
	}

	if cmd.Handler == nil {
		t.Error("expected command handler to be set, got nil")
	}

	if !cmd.IsPublic {
		t.Error("expected command to be public")
	}
}
