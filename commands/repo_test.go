package commands

import (
	"testing"
)

func TestRepoCommandRegistration(t *testing.T) {
	cmd, ok := Get("repo")
	if !ok {
		t.Fatal("expected 'repo' command to be registered")
	}

	if cmd.Category != "info" {
		t.Errorf("expected category 'info', got %q", cmd.Category)
	}

	if cmd.Handler == nil {
		t.Error("expected command handler to be set, got nil")
	}

	if !cmd.IsPublic {
		t.Error("expected command to be public")
	}

	foundAlias := false
	for _, a := range cmd.Aliases {
		if a == "sc" {
			foundAlias = true
			break
		}
	}
	if !foundAlias {
		t.Error("expected alias 'sc' to be registered")
	}
}
