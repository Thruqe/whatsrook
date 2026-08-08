package whatsrook

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"whatsrook/wa-core"
	"whatsrook/wa-core/types/events"
)

func (b *Bot) runPairCode(ctx context.Context) error {
	slog.Info("requesting pair code", "phone", b.cfg.Session)

	paired := make(chan error, 1)
	b.client.AddEventHandler(func(evt any) {
		switch v := evt.(type) {
		case *events.PairSuccess:
			paired <- nil
		case *events.PairError:
			slog.Error("pair error:", "err", v.Error)
			paired <- v.Error
		}
	})

	if !b.client.IsConnected() {
		if err := b.client.Connect(); err != nil {
			return err
		}
	}

	var pairType whatsmeow.PairClientType
	var clientDisplay string

	switch b.cfg.ClientType {
	case ClientAndroid:
		pairType = whatsmeow.PairClientAndroid
		clientDisplay = "Chrome (Android)"
	case ClientIos:
		pairType = whatsmeow.PairClientChrome
		clientDisplay = "Chrome (iOS)"
	default:
		pairType = whatsmeow.PairClientChrome
		clientDisplay = "Chrome (Linux)"
	}

	code, err := b.client.PairPhone(ctx, b.cfg.Session, true, pairType, clientDisplay)
	if err != nil {
		return fmt.Errorf("pair code failed: %w", err)
	}
	slog.Debug("pair code issued", "code", code)
	slog.Info(fmt.Sprintf("PAIR CODE: %s", code))
	b.hub.Broadcast(EventMessage{
		Kind:    EventPairCode,
		Payload: PairCodePayload{Code: code},
	})

	go func() {
		pairDeadline := time.After(60 * time.Second)
		select {
		case err := <-paired:
			if err != nil {
				slog.Error("pair error", "err", err)
			} else {
				slog.Info("paired successfully")
			}
		case <-pairDeadline:
			slog.Warn("pairing timed out")
		case <-ctx.Done():
			return
		}
	}()

	return nil
}
