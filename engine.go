package main

import (
	"context"
	"errors"
	"log/slog"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waCompanionReg"
	"go.mau.fi/whatsmeow/store"
)

// ErrPairTimeout is returned when Whatsmoew fails to complete the pairing
// handshake within the designated deadline.
// This happens when you request pair code and leave it stale
var ErrPairTimeout = errors.New("pairing timed out")

// Bot manages the state and lifecycle of the WhatsApp client, bridging incoming
// platform events with the central event hub and handling control messages.
type Bot struct {
	client *whatsmeow.Client
	hub    *Hub
	cli    CliArgs
}

// newBot initializes and returns a new Bot instance with the provided whatsmeow Client,
// central Hub, and command-line execution parameters.
func newBot(client *whatsmeow.Client, hub *Hub, cli CliArgs) *Bot {
	return &Bot{client: client, hub: hub, cli: cli}
}

// run configures the client properties, attaches event handlers, manages initial connection
// or pairing flows, and processes incoming control commands until the context is canceled.
func (b *Bot) run(ctx context.Context) error {
	b.client.AddEventHandler(func(evt any) {
		b.handleWAEvent(evt)
	})

	switch b.cli.Client {
	case ClientAndroid:
		store.DeviceProps.PlatformType = waCompanionReg.DeviceProps_ANDROID_PHONE.Enum()
		store.DeviceProps.Os = new("Android")
	case ClientIos:
		store.DeviceProps.PlatformType = waCompanionReg.DeviceProps_IOS_PHONE.Enum()
		store.DeviceProps.Os = new("iOS")
	default: // ClientChrome
		store.DeviceProps.PlatformType = waCompanionReg.DeviceProps_CHROME.Enum()
		store.DeviceProps.Os = new("Linux")
	}

	if b.client.Store.ID == nil {
		if b.cli.Pair {
			if err := b.runPairCode(ctx); err != nil {
				return err
			}
		} else {
			go func() {
				if err := b.runQR(ctx); err != nil {
					slog.Error("runQR failed", "err", err)
				}
			}()
		}
	} else {
		if err := b.client.Connect(); err != nil {
			return err
		}
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case ctrl := <-b.hub.Control:
			ack := b.handleControl(ctx, ctrl)
			b.hub.Broadcast(ack)
		}
	}
}
