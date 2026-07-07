package commands

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types/events"
)

const audioDir = "./media/call-audio"

// HandlePendingAudioReply handles the audio flow. It supports:
//   - Sending an audio file directly.
//   - Replying to an existing audio file with the text "save".
func HandlePendingAudioReply(ctx context.Context, client *whatsmeow.Client, evt *events.Message) bool {
	sender := evt.Info.Sender

	// 1. Check if the user even has a pending call request setup
	p, ok := peekPending(sender)
	if !ok {
		return false
	}

	var audioMsg *waE2E.AudioMessage
	saveRequested := false

	// 2. Check if the message ITSELF is an audio message
	if msg := evt.Message.GetAudioMessage(); msg != nil {
		log.Printf("[DEBUG] Detected direct audio message from %s", sender.String())
		audioMsg = msg
		saveRequested = isSaveText(getDirectMessageText(evt.Message))
	} else {
		// 3. Otherwise, check if it's a text reply quoting an audio message
		extText := evt.Message.GetExtendedTextMessage()
		if extText != nil && isSaveText(extText.GetText()) {
			log.Printf("[DEBUG] Detected text message containing 'save' from %s. Checking if it quotes audio...", sender.String())

			// Extract the quoted/context message
			if ctxInfo := extText.GetContextInfo(); ctxInfo != nil && ctxInfo.QuotedMessage != nil {
				if quotedAudio := ctxInfo.QuotedMessage.GetAudioMessage(); quotedAudio != nil {
					log.Printf("[DEBUG] Success! Found quoted audio message in the reply from %s", sender.String())
					audioMsg = quotedAudio
					saveRequested = true
				}
			}
		}
	}

	// If no audio message was resolved, let the normal dispatch loop continue
	if audioMsg == nil {
		log.Printf("[DEBUG] Message from %s did not provide or quote an audio message. Skipping pending intercept.", sender.String())
		return false
	}

	// We found valid audio to complete the flow! Consume the pending state.
	popPending(sender)

	go func() {
		log.Printf("[DEBUG] Downloading audio payload for %s...", sender.String())
		data, err := client.Download(ctx, audioMsg)
		if err != nil {
			log.Printf("[ERROR] Download failed: %v", err)
			sendTextRaw(ctx, client, evt.Info.Chat, fmt.Sprintf("failed to download audio: %v", err))
			return
		}

		if err := os.MkdirAll(audioDir, 0755); err != nil {
			log.Printf("[ERROR] Failed creating directory: %v", err)
			sendTextRaw(ctx, client, evt.Info.Chat, fmt.Sprintf("failed to prepare storage: %v", err))
			return
		}

		ext := extensionFor(audioMsg.GetMimetype())
		path := filepath.Join(audioDir, sanitizeJID(sender.String())+ext)
		if err := os.WriteFile(path, data, 0644); err != nil {
			log.Printf("[ERROR] File save failed: %v", err)
			sendTextRaw(ctx, client, evt.Info.Chat, fmt.Sprintf("failed to save audio: %v", err))
			return
		}

		cctx := &Context{
			Ctx:    ctx,
			Client: client,
			Evt:    evt,
			Chat:   evt.Info.Chat,
			Sender: sender,
		}

		if saveRequested {
			log.Printf("[DEBUG] Persisting audio choice to long-term memory storage for user: %s", sender.String())
			if err := saveAudio(cctx, sender, path); err != nil {
				log.Printf("[ERROR] saveAudio failed: %v", err)
				logHandlerErr("call-audio-save", err)
			}
		}

		log.Printf("[DEBUG] Triggering outgoing call to target: %s with media: %s", p.Target, path)
		if err := placeCallWithAudio(cctx, p.Target, path); err != nil {
			log.Printf("[ERROR] placeCallWithAudio failed: %v", err)
			logHandlerErr("call", err)
		}
	}()

	return true
}
