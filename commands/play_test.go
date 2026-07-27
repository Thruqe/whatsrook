package commands

import (
	"testing"
)

func TestPlayCommandRegistration(t *testing.T) {
	cmd, ok := Get("play")
	if !ok {
		t.Fatal("expected 'play' command to be registered")
	}

	if cmd.Category != "media" {
		t.Errorf("expected category 'media', got %q", cmd.Category)
	}

	if cmd.Handler == nil {
		t.Error("expected command handler to be set, got nil")
	}

	if !cmd.IsPublic {
		t.Error("expected command to be public")
	}
}
