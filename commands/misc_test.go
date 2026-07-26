package commands

import (
	"testing"
)

func TestMiscCommandsRegistration(t *testing.T) {
	miscCmds := []string{"save", "weather", "urban", "qrcode", "shorturl", "stkinfo", "calc"}

	for _, name := range miscCmds {
		cmd, ok := Get(name)
		if !ok {
			t.Fatalf("expected misc command %q to be registered", name)
		}
		if cmd.Category != "misc" {
			t.Errorf("expected category 'misc' for %q, got %q", name, cmd.Category)
		}
		if cmd.Handler == nil {
			t.Errorf("expected command handler for %q to be set", name)
		}
		if !cmd.IsPublic {
			t.Errorf("expected command %q to be public", name)
		}
	}
}

func TestEvalMathExpr(t *testing.T) {
	tests := []struct {
		expr    string
		want    float64
		wantErr bool
	}{
		{"2 + 3", 5, false},
		{"10 - 4", 6, false},
		{"3 * 4", 12, false},
		{"20 / 4", 5, false},
		{"(2 + 3) * 4", 20, false},
		{"10 / 0", 0, true},
		{"invalid", 0, true},
	}

	for _, tt := range tests {
		got, err := evalMathExpr(tt.expr)
		if (err != nil) != tt.wantErr {
			t.Errorf("evalMathExpr(%q) error = %v, wantErr %v", tt.expr, err, tt.wantErr)
			continue
		}
		if !tt.wantErr && got != tt.want {
			t.Errorf("evalMathExpr(%q) = %v, want %v", tt.expr, got, tt.want)
		}
	}
}
