package logger

import (
	"log/slog"
	"os"
	"strings"
	"testing"
)

func TestDebugLogOnlyErrors(t *testing.T) {
	// Remove existing debug.log if any
	os.Remove("debug.log")
	defer os.Remove("debug.log")

	if err := InitLogger(true); err != nil {
		t.Fatalf("InitLogger failed: %v", err)
	}
	defer Close()

	slog.Info("this is an info log")
	slog.Warn("this is a warn log")
	slog.Debug("this is a debug log")
	slog.Error("this is an error log")

	wml := WhatsmeowStyle("TestMod", "DEBUG", false)
	wml.Infof("whatsmeow info log")
	wml.Warnf("whatsmeow warn log")
	wml.Errorf("whatsmeow error log")

	// Close file to flush buffer
	Close()

	content, err := os.ReadFile("debug.log")
	if err != nil {
		t.Fatalf("failed to read debug.log: %v", err)
	}

	strContent := string(content)

	if strings.Contains(strContent, "this is an info log") {
		t.Errorf("debug.log contains info log")
	}
	if strings.Contains(strContent, "this is a warn log") {
		t.Errorf("debug.log contains warn log")
	}
	if strings.Contains(strContent, "this is a debug log") {
		t.Errorf("debug.log contains debug log")
	}
	if !strings.Contains(strContent, "this is an error log") {
		t.Errorf("debug.log missing error log")
	}

	if strings.Contains(strContent, "whatsmeow info log") {
		t.Errorf("debug.log contains whatsmeow info log")
	}
	if strings.Contains(strContent, "whatsmeow warn log") {
		t.Errorf("debug.log contains whatsmeow warn log")
	}
	if !strings.Contains(strContent, "whatsmeow error log") {
		t.Errorf("debug.log missing whatsmeow error log")
	}
}
