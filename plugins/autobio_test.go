package plugins

import (
	"strings"
	"testing"
)

func TestGenerateBioText(t *testing.T) {
	bio := generateBioText("UTC")
	if !strings.HasPrefix(bio, "⏰ ") {
		t.Errorf("generateBioText() = %q; expected to start with '⏰ '", bio)
	}
	if !strings.Contains(bio, "|") {
		t.Errorf("generateBioText() = %q; expected to contain '|' separator", bio)
	}

	bioLagos := generateBioText("Africa/Lagos")
	if !strings.HasPrefix(bioLagos, "⏰ ") {
		t.Errorf("generateBioText(Africa/Lagos) = %q; expected to start with '⏰ '", bioLagos)
	}
}
