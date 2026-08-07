package plugins

import (
	"testing"
)

func TestCommandRegistrationAndAliases(t *testing.T) {
	all := All()
	if len(all) == 0 {
		t.Fatalf("expected registered commands, got none")
	}

	seenNames := make(map[string]string)
	for _, cmd := range all {
		if cmd.Name == "" {
			t.Errorf("found command with empty name")
			continue
		}
		if existing, found := seenNames[cmd.Name]; found {
			t.Errorf("duplicate primary command name '%s' found in '%s'", cmd.Name, existing)
		} else {
			seenNames[cmd.Name] = cmd.Name
		}
	}

	// Verify pin command is registered as pin and has no alias conflict
	pinCmd, found := Get("pin")
	if !found || pinCmd == nil || pinCmd.Name != "pin" {
		t.Errorf("expected Get('pin') to return primary pin command, got %v", pinCmd)
	}

	// Verify pinterest command is registered as pinterest
	pntCmd, found := Get("pinterest")
	if !found || pntCmd == nil || pntCmd.Name != "pinterest" {
		t.Errorf("expected Get('pinterest') to return pinterest command, got %v", pntCmd)
	}
}
