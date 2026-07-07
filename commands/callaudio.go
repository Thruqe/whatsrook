package commands

import (
	"fmt"
	"sync"

	"github.com/Thruqe/whatsrook/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
)

// pendingCall tracks a user mid-flow: they ran !call <target> and we're waiting
// for them to reply with an audio message.
type pendingCall struct {
	Target string
}

var (
	pendingMu sync.Mutex
	pending   = map[types.JID]*pendingCall{}
)

func setPending(sender types.JID, p *pendingCall) {
	pendingMu.Lock()
	defer pendingMu.Unlock()
	pending[sender] = p
}

func peekPending(sender types.JID) (*pendingCall, bool) {
	pendingMu.Lock()
	defer pendingMu.Unlock()
	p, ok := pending[sender]
	return p, ok
}

func popPending(sender types.JID) (*pendingCall, bool) {
	pendingMu.Lock()
	defer pendingMu.Unlock()
	p, ok := pending[sender]
	if ok {
		delete(pending, sender)
	}
	return p, ok
}

// audioStore extracts the concrete *sqlstore.SQLStore from a Context's client,
// since PutCallAudioConfig/GetCallAudioConfig are project-specific, not part of
// any whatsmeow store interface.
func audioStore(ctx *Context) (*sqlstore.SQLStore, error) {
	s, ok := ctx.Client.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return nil, fmt.Errorf("unexpected store implementation")
	}
	return s, nil
}

func getSavedAudio(ctx *Context, sender types.JID) (string, bool) {
	s, err := audioStore(ctx)
	if err != nil {
		return "", false
	}
	path, err := s.GetCallAudioConfig(ctx.Ctx, sender)
	if err != nil || path == "" {
		return "", false
	}
	return path, true
}

func saveAudio(ctx *Context, sender types.JID, path string) error {
	s, err := audioStore(ctx)
	if err != nil {
		return err
	}
	return s.PutCallAudioConfig(ctx.Ctx, sender, path)
}
