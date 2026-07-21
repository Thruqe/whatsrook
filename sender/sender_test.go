package sender

import "testing"

func TestFormatTextResponseRaw(t *testing.T) {
	// Monospace formatting check
	input := "Hello World"
	expected := "𝙷𝚎𝚕𝚕𝚘 𝚆𝚘𝚛𝚕𝚍"
	actual := FormatTextResponseRaw(input)
	if actual != expected {
		t.Errorf("Expected %q, got %q", expected, actual)
	}

	// Should format cleanly even if backticks are present
	alreadyFormatted := "```Hello World```"
	actual2 := FormatTextResponseRaw(alreadyFormatted)
	if actual2 != expected {
		t.Errorf("Expected %q to remain unchanged, but got %q", expected, actual2)
	}

	// Asterisks and emojis removal check
	inputWithAsterisks := "*Hello*  World"
	expectedCleaned := "𝙷𝚎𝚕𝚕𝚘  𝚆𝚘𝚛𝚕𝚍" // emoji removed, asterisks removed
	actual3 := FormatTextResponseRaw(inputWithAsterisks)
	if actual3 != expectedCleaned {
		t.Errorf("Expected %q, got %q", expectedCleaned, actual3)
	}
}
