package whatsrook

import (
	"context"
	"fmt"
	"log/slog"
)

func (b *Bot) runQR(ctx context.Context) error {
	qrChan, _ := b.client.GetQRChannel(ctx)
	if !b.client.IsConnected() {
		if err := b.client.Connect(); err != nil {
			return err
		}
	}
	for evt := range qrChan {
		if evt.Event == "code" {
			if b.cfg.QRCode {
				fmt.Println("QR code:", evt.Code)
			}
			b.hub.Broadcast(EventMessage{
				Kind:    EventPairQR,
				Payload: PairQRPayload{Code: evt.Code},
			})
		} else {
			slog.Debug("qr channel event", "event", evt.Event)
		}
	}
	return nil
}
