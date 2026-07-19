package logger

import (
	"os"
	"testing"
)

func TestInitLogger(t *testing.T) {
	// Initialize logger in non-verbose mode
	err := InitLogger(false)
	if err != nil {
		t.Fatalf("Failed to initialize logger: %v", err)
	}
	defer Close()

	// Verify that debug.log was created
	if _, err := os.Stat("debug.log"); os.IsNotExist(err) {
		t.Error("debug.log file was not created")
	}
}
