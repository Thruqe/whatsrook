package commands

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"whatsrook/utils"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

const audioDir = "./media/call-audio"

// HandlePendingAudioReply handles the audio call-setup flow. It supports:
//   - Sending an audio file directly.
//   - Replying "save" to a message that quotes an audio file.
func HandlePendingAudioReply(ctx context.Context, client *whatsmeow.Client, evt *events.Message) bool {
	sender := evt.Info.Sender

	p, ok := peekPending(sender)
	if !ok {
		return false
	}

	var audioMsg *waE2E.AudioMessage
	saveRequested := false

	if msg := evt.Message.GetAudioMessage(); msg != nil {
		log.Printf("[DEBUG] Detected direct audio message from %s", sender.String())
		audioMsg = msg
		saveRequested = utils.IsSaveText(utils.GetDirectMessageText(evt.Message))
	} else if extText := evt.Message.GetExtendedTextMessage(); extText != nil && utils.IsSaveText(extText.GetText()) {
		log.Printf("[DEBUG] Detected text message containing 'save' from %s. Checking if it quotes audio...", sender.String())
		if ctxInfo := extText.GetContextInfo(); ctxInfo != nil && ctxInfo.QuotedMessage != nil {
			if quotedAudio := ctxInfo.QuotedMessage.GetAudioMessage(); quotedAudio != nil {
				log.Printf("[DEBUG] Success! Found quoted audio message in the reply from %s", sender.String())
				audioMsg = quotedAudio
				saveRequested = true
			}
		}
	}

	if audioMsg == nil {
		log.Printf("[DEBUG] Message from %s did not provide or quote an audio message. Skipping pending intercept.", sender.String())
		return false
	}

	popPending(sender)

	go func() {
		cctx := &Context{
			Ctx:    ctx,
			Client: client,
			Evt:    evt,
			Chat:   evt.Info.Chat,
			Sender: sender,
		}
		handleAudioDownload(ctx, client, cctx, sender, evt, audioMsg, p, saveRequested)
	}()

	return true
}

func handleAudioDownload(ctx context.Context, client *whatsmeow.Client, cctx *Context, sender types.JID, evt *events.Message, audioMsg *waE2E.AudioMessage, p *pendingCall, saveRequested bool) {
	log.Printf("[DEBUG] Downloading audio payload for %s...", sender.String())
	data, err := client.Download(ctx, audioMsg)
	if err != nil {
		log.Printf("[ERROR] Download failed: %v", err)
		if sendErr := sendTextRaw(ctx, client, evt.Info.Chat, fmt.Sprintf("failed to download audio: %v", err)); sendErr != nil {
			log.Printf("[ERROR] failed to notify user: %v", sendErr)
		}
		return
	}

	if err := os.MkdirAll(audioDir, 0755); err != nil {
		log.Printf("[ERROR] Failed creating directory: %v", err)
		if sendErr := sendTextRaw(ctx, client, evt.Info.Chat, fmt.Sprintf("failed to prepare storage: %v", err)); sendErr != nil {
			log.Printf("[ERROR] failed to notify user: %v", sendErr)
		}
		return
	}

	ext := utils.ExtensionFor(audioMsg.GetMimetype())
	path := filepath.Join(audioDir, utils.SanitizeJID(sender.String())+ext)
	if err := os.WriteFile(path, data, 0644); err != nil {
		log.Printf("[ERROR] File save failed: %v", err)
		if sendErr := sendTextRaw(ctx, client, evt.Info.Chat, fmt.Sprintf("failed to save audio: %v", err)); sendErr != nil {
			log.Printf("[ERROR] failed to notify user: %v", sendErr)
		}
		return
	}

	// meowcaller's OpusFile can't reliably play back WhatsApp's Ogg/Opus voice
	// notes (silent output despite RTP flowing) — transcode to MP3 via ffmpeg
	// so every call source is a format meowcaller actually plays correctly.
	path, err = utils.TranscodeToMP3(path)
	if err != nil {
		log.Printf("[ERROR] Transcode failed: %v", err)
		if sendErr := sendTextRaw(ctx, client, evt.Info.Chat, fmt.Sprintf("failed to process audio: %v", err)); sendErr != nil {
			log.Printf("[ERROR] failed to notify user: %v", sendErr)
		}
		return
	}

	if saveRequested {
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
}
