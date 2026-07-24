package font

import (
	"testing"
)

func TestDefaultFontSmallCaps(t *testing.T) {
	style := GetStyle()
	if style != "small-caps" {
		t.Errorf("expected default style to be 'small-caps', got %q", style)
	}

	input := "abcdefghijklmnopqrstuvwxyz"
	expected := "ᴀʙᴄᴅᴇғɢʜɪᴊᴋʟᴍɴᴏᴘǫʀsᴛᴜᴠᴡxʏᴢ"
	actual := Convert(input)
	if actual != expected {
		t.Errorf("expected Convert(%q) = %q, got %q", input, expected, actual)
	}

	upperInput := "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	actualUpper := Convert(upperInput)
	if actualUpper != expected {
		t.Errorf("expected Convert(%q) = %q, got %q", upperInput, expected, actualUpper)
	}
}
