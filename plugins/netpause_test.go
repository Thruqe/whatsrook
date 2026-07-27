package commands

import (
	"testing"
)

func TestNetPauseCommandRegistration(t *testing.T) {
	cmd, ok := Get("netpause")
	if !ok {
		t.Fatalf("expected netpause command to be registered")
	}

	if cmd.IsPublic {
		t.Errorf("expected netpause command to be restricted (IsPublic=false)")
	}
}
