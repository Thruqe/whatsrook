package meowcaller

import (
	"context"
	"errors"
	"testing"

	"github.com/rs/zerolog"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store"
	waLog "go.mau.fi/whatsmeow/util/log"
)

func TestInstallCallAckHookMatchesPinnedUpstreamLayout(t *testing.T) {
	wa := whatsmeow.NewClient(&store.Device{}, waLog.Noop)
	client := &Client{wa: wa, log: zerolog.Nop()}
	eng := newEngine(client)
	if err := eng.installCallAckHook(); err != nil {
		t.Fatalf("install raw call adapter: %v", err)
	}
}

func TestInstallCallAckHookRejectsMissingClient(t *testing.T) {
	client := &Client{log: zerolog.Nop()}
	eng := newEngine(client)
	if err := eng.installCallAckHook(); err == nil {
		t.Fatal("raw call adapter accepted a missing whatsmeow client")
	}
}

func TestGroupFeaturesRejectUnavailableRawAdapter(t *testing.T) {
	client := &Client{log: zerolog.Nop()}
	eng := newEngine(client)
	eng.rawCallHookErr = errors.New("upstream layout changed")
	if _, err := eng.placeGroupCall(
		context.Background(),
		[]string{"1", "2"},
		GroupCallOptions{},
	); err == nil {
		t.Fatal("group call continued without its raw call adapter")
	}
}
