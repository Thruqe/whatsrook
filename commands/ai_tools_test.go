package commands

import (
	"testing"
)

func TestAiBotToolsRegistration(t *testing.T) {
	tools := []string{"send", "edit", "delete", "ffmpeg", "fetch"}

	for _, toolName := range tools {
		cmd, ok := Get(toolName)
		if !ok {
			t.Fatalf("expected tool command %q to be registered", toolName)
		}
		if cmd.Name != toolName && toolName != "delete" {
			t.Errorf("expected command name %q, got %q", toolName, cmd.Name)
		}
	}

	sendCmd, _ := Get("send")
	if !sendCmd.HideFromMenu {
		t.Error("expected 'send' command to have HideFromMenu = true")
	}

	editCmd, _ := Get("edit")
	if !editCmd.HideFromMenu {
		t.Error("expected 'edit' command to have HideFromMenu = true")
	}

	ffmpegCmd, _ := Get("ffmpeg")
	if !ffmpegCmd.HideFromMenu {
		t.Error("expected 'ffmpeg' command to have HideFromMenu = true")
	}
	if ffmpegCmd.IsPublic {
		t.Error("expected 'ffmpeg' command to have IsPublic = false (restricted)")
	}
}
