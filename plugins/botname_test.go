package plugins

import (
	"context"
	"os"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

func TestReconfigureCommandRegistered(t *testing.T) {
	cmd, ok := Get("reconfigure")
	if !ok || cmd == nil {
		t.Fatalf("expected 'reconfigure' command to be registered")
	}
	if cmd.Name != "reconfigure" {
		t.Errorf("expected command name 'reconfigure', got %q", cmd.Name)
	}

	aliasCmd, aliasOk := Get("reconfig")
	if !aliasOk || aliasCmd == nil {
		t.Fatalf("expected alias 'reconfig' to resolve to command")
	}
}

func TestWizardSessionTTL(t *testing.T) {
	key := "test_chat@g.us:test_sender@s.whatsapp.net"

	botWizardMu.Lock()
	pendingWizardState[key] = wizardSession{
		Step:      "name",
		UpdatedAt: time.Now().Add(-10 * time.Minute), // expired
	}
	botWizardMu.Unlock()

	dummyEvt := &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{
				Chat:   types.NewJID("test_chat", "g.us"),
				Sender: types.NewJID("test_sender", "s.whatsapp.net"),
			},
		},
		Message: &waE2E.Message{
			Conversation: proto.String("hello"),
		},
	}

	_ = HandlePendingBotCustomizationReply(context.Background(), nil, dummyEvt)

	botWizardMu.RLock()
	_, inWizard := pendingWizardState[key]
	botWizardMu.RUnlock()

	if inWizard {
		t.Errorf("expected expired wizard session to be deleted")
	}
}

func TestProcessAndSaveThumbnail_EmptyData(t *testing.T) {
	_, err := ProcessAndSaveThumbnail(context.Background(), "auth/test_session", []byte{}, false)
	if err == nil {
		t.Errorf("expected error when processing empty thumbnail data, got nil")
	}
}

func TestProcessAndSaveThumbnail_Image(t *testing.T) {
	authDir := "auth/test_session"
	// 1x1 GIF / minimal byte payload
	data := []byte("GIF89a\x01\x00\x01\x00\x80\x00\x00\xff\xff\xff\x00\x00\x00!\xf9\x04\x01\x00\x00\x00\x00,\x00\x00\x00\x00\x01\x00\x01\x00\x00\x02\x02D\x01\x00;")
	path, err := ProcessAndSaveThumbnail(context.Background(), authDir, data, false)
	if err != nil {
		t.Fatalf("unexpected error processing thumbnail: %v", err)
	}
	if path == "" {
		t.Fatalf("expected non-empty target path")
	}
	defer os.RemoveAll(authDir)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Errorf("expected created thumbnail file at %s, but file does not exist", path)
	}
}
