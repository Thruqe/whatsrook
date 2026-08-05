package plugins

import (
	"testing"

	"whatsrook/utils"
)

func TestTTSCommandRegistered(t *testing.T) {
	cmd, ok := Get("tts")
	if !ok || cmd == nil {
		t.Fatalf("expected 'tts' command to be registered")
	}
	if cmd.Name != "tts" {
		t.Errorf("expected command name 'tts', got %q", cmd.Name)
	}

	aliasCmd, aliasOk := Get("say")
	if !aliasOk || aliasCmd == nil {
		t.Fatalf("expected alias 'say' to resolve to tts command")
	}
}

func TestIsKnownLanguageCode(t *testing.T) {
	validCodes := []string{"en", "es", "fr", "de", "ar", "yo", "ha", "ig", "ja"}
	for _, code := range validCodes {
		if !utils.IsKnownLanguageCode(code) {
			t.Errorf("expected language code %q to be known", code)
		}
	}

	invalidCodes := []string{"xyz1234", "invalid_code", ""}
	for _, code := range invalidCodes {
		if utils.IsKnownLanguageCode(code) {
			t.Errorf("expected language code %q to be unknown", code)
		}
	}
}
