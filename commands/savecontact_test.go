package commands

import (
	"testing"
)

func TestSaveContactCommandRegistration(t *testing.T) {
	cmd, ok := Get("savecontact")
	if !ok {
		t.Fatal("expected 'savecontact' command to be registered")
	}

	if cmd.Category != "user" {
		t.Errorf("expected category 'user', got %q", cmd.Category)
	}

	if !cmd.IsPublic {
		t.Error("expected command to be public")
	}

	if cmd.Handler == nil {
		t.Error("expected command handler to be set, got nil")
	}

	foundAlias := false
	for _, a := range cmd.Aliases {
		if a == "addcontact" {
			foundAlias = true
			break
		}
	}
	if !foundAlias {
		t.Error("expected alias 'addcontact' to be registered")
	}
}
