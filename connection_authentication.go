// Pair-code and QR-code authentication flows for device registration.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types/events"
)

func (b *Bot) runPairCode(ctx context.Context) error {
	slog.Info("requesting pair code", "phone", b.cli.Session)

	paired := make(chan error, 1)
	b.client.AddEventHandler(func(evt any) {
		switch v := evt.(type) {
		case *events.PairSuccess:
			paired <- nil
		case *events.PairError:
			paired <- fmt.Errorf("pair error: %w", v.Error)
		}
	})

	if !b.client.IsConnected() {
		if err := b.client.Connect(); err != nil {
			return err
		}
	}

	var pairType whatsmeow.PairClientType
	var clientDisplay string

	switch b.cli.Client {
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

	code, err := b.client.PairPhone(ctx, b.cli.Session, true, pairType, clientDisplay)
	if err != nil {
		return fmt.Errorf("pair code failed: %w", err)
	}
	slog.Info("pair code issued", "code", code)
	fmt.Printf("Enter this code on your phone: %s\n", code)
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

func (b *Bot) runQR(ctx context.Context) error {
	qrChan, _ := b.client.GetQRChannel(ctx)
	if !b.client.IsConnected() {
		if err := b.client.Connect(); err != nil {
			return err
		}
	}
	for evt := range qrChan {
		if evt.Event == "code" {
			if b.cli.QRCode {
				fmt.Println("QR code:", evt.Code)
			}
			b.hub.Broadcast(EventMessage{
				Kind:    EventPairQR,
				Payload: PairQRPayload{Code: evt.Code},
			})
		} else {
			slog.Info("qr channel event", "event", evt.Event)
		}
	}
	return nil
}
