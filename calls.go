package main

import (
	"context"
	"fmt"

	"github.com/purpshell/meowcaller"
	"github.com/rs/zerolog"
	"go.mau.fi/whatsmeow"
)

// InitiateCall places a managed 1:1 call to target and wires mic <-> speaker.
// Call must be made with an already-connected whatsmeow.Client.
func InitiateCall(ctx context.Context, wa *whatsmeow.Client, target string) error {
	logger := zerolog.Ctx(ctx)

	client := meowcaller.NewClient(wa, meowcaller.WithLogger(*logger))

	call, err := client.Call(ctx, target)
	if err != nil {
		return fmt.Errorf("place call: %w", err)
	}

	call.OnReady(func() {
		logger.Info().Str("call_id", call.ID()).Msg("media flowing")
	})
	call.OnEnd(func(reason string) {
		logger.Info().Str("call_id", call.ID()).Str("reason", reason).Msg("call ended")
	})

	logger.Info().Str("call_id", call.ID()).Msg("call placed; waiting for peer to answer")
	return nil
}
