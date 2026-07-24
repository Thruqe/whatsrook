package commands

import (
	"testing"
)

func TestWelcomeGoodbyeRegistration(t *testing.T) {
	welc, ok := Get("welcome")
	if !ok {
		t.Fatal("expected 'welcome' command to be registered")
	}
	if !welc.GroupOnly {
		t.Error("expected 'welcome' to be group only")
	}

	bye, ok := Get("goodbye")
	if !ok {
		t.Fatal("expected 'goodbye' command to be registered")
	}
	if !bye.GroupOnly {
		t.Error("expected 'goodbye' to be group only")
	}
}
