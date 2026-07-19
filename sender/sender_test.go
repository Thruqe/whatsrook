package sender

import "testing"

func TestFormatTextResponseRaw(t *testing.T) {
	// Monospace formatting check
	input := "Hello World"
	expected := "```\nHello World\n```"
	actual := FormatTextResponseRaw(input)
	if actual != expected {
		t.Errorf("Expected %q, got %q", expected, actual)
	}

	// Should not double wrap if already formatted
	alreadyFormatted := "```\nHello World\n```"
	actual2 := FormatTextResponseRaw(alreadyFormatted)
	if actual2 != alreadyFormatted {
		t.Errorf("Expected %q to remain unchanged, but got %q", alreadyFormatted, actual2)
	}

	// Asterisks and emojis removal check
	inputWithAsterisks := "*Hello* 👋 World"
	expectedCleaned := "```\nHello  World\n```" // emoji removed, asterisks removed
	actual3 := FormatTextResponseRaw(inputWithAsterisks)
	if actual3 != expectedCleaned {
		t.Errorf("Expected %q, got %q", expectedCleaned, actual3)
	}
}
