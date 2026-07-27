package commands

import (
	"testing"
)

func TestAntiMsgCommandRegistration(t *testing.T) {
	cmd, ok := Get("antimsg")
	if !ok {
		t.Fatal("expected 'antimsg' command to be registered")
	}

	if cmd.Category != "group" {
		t.Errorf("expected category 'group', got %q", cmd.Category)
	}

	if !cmd.GroupOnly {
		t.Error("expected antimsg command to be group only")
	}

	if cmd.IsPublic {
		t.Error("expected antimsg command to be restricted (IsPublic: false)")
	}
}
