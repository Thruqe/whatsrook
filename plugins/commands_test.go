package commands

import (
	"testing"
)

func TestVisibleAndAllNoPanicOnNilOrCaseMismatch(t *testing.T) {
	// Register commands with varied casing and nil checks
	Register(&Command{
		Name:         "TestCmdUpper",
		Description:  "Test Upper Case",
		Category:     "test",
		HideFromMenu: false,
	})

	Register(&Command{
		Name:         "HiddenCmd",
		Description:  "Test Hidden",
		Category:     "test",
		HideFromMenu: true,
	})

	// Test Nil registration
	Register(nil)

	// Ensure Visible() does not panic and contains TestCmdUpper
	visible := Visible()
	foundUpper := false
	for _, c := range visible {
		if c.Name == "TestCmdUpper" {
			foundUpper = true
		}
		if c.Name == "HiddenCmd" {
			t.Errorf("HiddenCmd should not be visible in menu")
		}
	}
	if !foundUpper {
		t.Errorf("expected TestCmdUpper to be in visible commands")
	}

	// Ensure All() returns non-nil elements
	allCmds := All()
	for _, c := range allCmds {
		if c == nil {
			t.Fatalf("All() returned a nil command entry")
		}
	}
}
