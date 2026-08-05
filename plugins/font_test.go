package plugins

import (
	"testing"
)

func TestFancyCommandRegistered(t *testing.T) {
	cmd, ok := Get("fancy")
	if !ok || cmd == nil {
		t.Fatalf("expected 'fancy' command to be registered")
	}

	cmdList, okList := Get("fontlist")
	if !okList || cmdList == nil {
		t.Fatalf("expected 'fontlist' command to be registered")
	}

	aliasCmd, aliasOk := Get("style")
	if !aliasOk || aliasCmd == nil {
		t.Fatalf("expected 'style' alias to resolve to fancy command")
	}
}

func TestIndexedFontsCount(t *testing.T) {
	if len(indexedFonts) < 20 {
		t.Errorf("expected at least 20 font styles in indexedFonts, got %d", len(indexedFonts))
	}
	if indexedFonts[13].Key != "small-caps" {
		t.Errorf("expected font #14 (index 13) to be small-caps, got %s", indexedFonts[13].Key)
	}
}
