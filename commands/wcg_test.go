package commands

import (
	"testing"

	"go.mau.fi/whatsmeow/types"
)

func TestWCGRegistration(t *testing.T) {
	if cmd, ok := Get("wcg"); !ok || cmd == nil {
		t.Fatal("Expected 'wcg' command to be registered")
	}
	if cmd, ok := Get("wcgleaderboard"); !ok || cmd == nil {
		t.Fatal("Expected 'wcgleaderboard' command to be registered")
	}
}

func TestWCGEmojiDetection(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"😀", true},
		{"🔥🎉", true},
		{"hello 😀", false},
		{"apple", false},
		{"👍👍👍", true},
		{"   ", false},
	}

	for _, tt := range tests {
		result := isPureEmoji(tt.input)
		if result != tt.expected {
			t.Errorf("isPureEmoji(%q) = %v, expected %v", tt.input, result, tt.expected)
		}
	}
}

func TestWCGTurnTimeLimit(t *testing.T) {
	if limit := getTurnTimeLimit(3); limit != 30 {
		t.Errorf("Expected 30s limit for level 3, got %d", limit)
	}
	if limit := getTurnTimeLimit(16); limit != 6 {
		t.Errorf("Expected 6s limit for level 16, got %d", limit)
	}
	if limit := getTurnTimeLimit(10); limit < 6 || limit > 30 {
		t.Errorf("Expected limit between 6s and 30s for level 10, got %d", limit)
	}
}

func TestWCGCXPTitle(t *testing.T) {
	titles := map[int]string{
		50:    "🐣 Novice",
		200:   "🌱 Beginner",
		1000:  "⚔️ Pro",
		2500:  "🔥 Master",
		5000:  "⚡ Prolific",
		8000:  "🌟 Legend",
		15000: "👑 Legendary Master",
	}

	for xp, expected := range titles {
		title := getCXPTitle(xp)
		if title != expected {
			t.Errorf("getCXPTitle(%d) = %q, expected %q", xp, title, expected)
		}
	}
}

func TestWCGScrambleWord(t *testing.T) {
	word, scrambled := getRandomWord(5)
	if len(word) != 5 {
		t.Errorf("Expected word length 5, got %d (%s)", len(word), word)
	}
	if len(scrambled) != 5 {
		t.Errorf("Expected scrambled length 5, got %d (%s)", len(scrambled), scrambled)
	}
}

func TestWCGActiveCheck(t *testing.T) {
	chatKey := "12345@g.us"
	if IsWCGGameActive(chatKey) {
		t.Error("Expected no active WCG game initially")
	}

	wcgMu.Lock()
	wcgGames[chatKey] = &wcgGame{
		chatKey: chatKey,
		state:   wcgStateLobby,
		hostLID: types.JID{User: "123", Server: "s.whatsapp.net"},
	}
	wcgMu.Unlock()

	if !IsWCGGameActive(chatKey) {
		t.Error("Expected active WCG game after adding to wcgGames")
	}

	wcgMu.Lock()
	delete(wcgGames, chatKey)
	wcgMu.Unlock()

	if IsWCGGameActive(chatKey) {
		t.Error("Expected game to be removed")
	}
}
