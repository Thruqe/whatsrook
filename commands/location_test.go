package commands

import (
	"testing"
)

func TestLocButtonsCommandRegistration(t *testing.T) {
	cmd, ok := Get("locbuttons")
	if !ok {
		t.Fatal("expected 'locbuttons' command to be registered")
	}

	if cmd.Category != "example" {
		t.Errorf("expected category 'example', got %q", cmd.Category)
	}

	if cmd.Handler == nil {
		t.Error("expected command handler to be set, got nil")
	}

	if !cmd.IsPublic {
		t.Error("expected command to be public")
	}
}
