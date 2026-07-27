package commands

import (
	"testing"
)

func TestAntiSpamCommandRegistration(t *testing.T) {
	cmd, ok := Get("antispam")
	if !ok {
		t.Fatal("expected 'antispam' command to be registered")
	}

	if cmd.Category != "group" {
		t.Errorf("expected category 'group', got %q", cmd.Category)
	}

	if !cmd.GroupOnly {
		t.Error("expected command to be group only")
	}

	if cmd.IsPublic {
		t.Error("expected command to be restricted to admins/sudoers (IsPublic=false)")
	}

	if cmd.Handler == nil {
		t.Error("expected command handler to be set, got nil")
	}
}
