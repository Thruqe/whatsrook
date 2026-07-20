package commands

import (
	"testing"
)

func TestSelectListCommandRegistration(t *testing.T) {
	cmd, ok := Get("selectlist")
	if !ok {
		t.Fatal("expected 'selectlist' command to be registered")
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
