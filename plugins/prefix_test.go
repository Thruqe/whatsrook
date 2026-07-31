package commands

import (
	"testing"
)

func TestIsWordPrefix(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"jarvis", true},
		{"Jarvis", true},
		{"bot", true},
		{"rook123", true},
		{".", false},
		{"!", false},
		{".!", false},
		{"#", false},
	}

	for _, tt := range tests {
		got := isWordPrefix(tt.input)
		if got != tt.want {
			t.Errorf("isWordPrefix(%q) = %v; want %v", tt.input, got, tt.want)
		}
	}
}

func TestMatchesPrefix(t *testing.T) {
	tests := []struct {
		text   string
		prefix string
		want   bool
	}{
		{"jarvis ping", "jarvis", true},
		{"Jarvis ping", "jarvis", true},
		{"JARVIS ping", "jarvis", true},
		{"jarvis", "jarvis", true},
		{"Jarvis", "jarvis", true},
		{"jarvisization", "jarvis", false},
		{".ping", ".", true},
		{". ping", ".", true},
		{"!ping", "!", true},
	}

	for _, tt := range tests {
		got := matchesPrefix(tt.text, tt.prefix)
		if got != tt.want {
			t.Errorf("matchesPrefix(%q, %q) = %v; want %v", tt.text, tt.prefix, got, tt.want)
		}
	}
}
