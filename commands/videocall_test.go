package commands

import (
	"testing"
)

func TestVideoCallCommandsRegistration(t *testing.T) {
	vcCmd, ok := Get("videocall")
	if !ok {
		t.Fatal("expected 'videocall' command to be registered")
	}
	if vcCmd.Name != "videocall" {
		t.Errorf("expected command name 'videocall', got %q", vcCmd.Name)
	}
	if vcCmd.Category != "calls" {
		t.Errorf("expected category 'calls', got %q", vcCmd.Category)
	}

	setVcCmd, ok := Get("setvideocall")
	if !ok {
		t.Fatal("expected 'setvideocall' command to be registered")
	}
	if setVcCmd.Name != "setvideocall" {
		t.Errorf("expected command name 'setvideocall', got %q", setVcCmd.Name)
	}
	if setVcCmd.Category != "calls" {
		t.Errorf("expected category 'calls', got %q", setVcCmd.Category)
	}
}
