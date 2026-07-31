// Call audio reply – records and sends audio replies for incoming calls.
package commands

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"whatsrook/store/sqlstore"
	"whatsrook/utils"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

const (
	audioDir = "./media/call-audio"
	videoDir = "./media/call-video"
)

// HandlePendingAudioReply handles the audio and video call-setup flow. It supports:
//   - Sending an audio or video file directly.
//   - Replying "save" to a message that quotes an audio or video file.
func HandlePendingAudioReply(ctx context.Context, client *whatsmeow.Client, evt *events.Message) bool {
	sender := evt.Info.Sender

	p, ok := peekPending(sender)
	if !ok {
		return false
	}

	if p.Kind == sqlstore.CallMediaVideo {
		var videoMsg *waE2E.VideoMessage
		saveRequested := false

		if msg := evt.Message.GetVideoMessage(); msg != nil {
			slog.Debug("Detected direct video message", "sender", sender.String())
			videoMsg = msg
			saveRequested = utils.IsSaveText(utils.GetDirectMessageText(evt.Message))
		} else if extText := evt.Message.GetExtendedTextMessage(); extText != nil && utils.IsSaveText(extText.GetText()) {
			if ctxInfo := extText.GetContextInfo(); ctxInfo != nil && ctxInfo.QuotedMessage != nil {
				if quotedVideo := ctxInfo.QuotedMessage.GetVideoMessage(); quotedVideo != nil {
					videoMsg = quotedVideo
					saveRequested = true
				}
			}
		}

		if videoMsg == nil {
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
			handleVideoDownload(ctx, client, cctx, sender, evt, videoMsg, p, saveRequested)
		}()

		return true
	}

	var audioMsg *waE2E.AudioMessage
	saveRequested := false

	if msg := evt.Message.GetAudioMessage(); msg != nil {
		slog.Debug("Detected direct audio message", "sender", sender.String())
		audioMsg = msg
		saveRequested = utils.IsSaveText(utils.GetDirectMessageText(evt.Message))
	} else if extText := evt.Message.GetExtendedTextMessage(); extText != nil && utils.IsSaveText(extText.GetText()) {
		slog.Debug("Detected text message containing 'save', checking quoted audio...", "sender", sender.String())
		if ctxInfo := extText.GetContextInfo(); ctxInfo != nil && ctxInfo.QuotedMessage != nil {
			if quotedAudio := ctxInfo.QuotedMessage.GetAudioMessage(); quotedAudio != nil {
				slog.Debug("Found quoted audio message in reply", "sender", sender.String())
				audioMsg = quotedAudio
				saveRequested = true
			}
		}
	}

	if audioMsg == nil {
		slog.Debug("Message did not provide or quote an audio message, skipping pending intercept", "sender", sender.String())
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
	slog.Debug("Downloading audio payload", "sender", sender.String())
	data, err := client.Download(ctx, audioMsg)
	if err != nil {
		slog.Error("Download audio failed", "err", err)
		if sendErr := sendTextRaw(ctx, client, evt.Info.Chat, fmt.Sprintf("failed to download audio: %v", err)); sendErr != nil {
			slog.Error("failed to notify user", "sendErr", sendErr)
		}
		return
	}

	if err := os.MkdirAll(audioDir, 0755); err != nil {
		slog.Error("Failed creating audio directory", "err", err)
		if sendErr := sendTextRaw(ctx, client, evt.Info.Chat, fmt.Sprintf("failed to prepare storage: %v", err)); sendErr != nil {
			slog.Error("failed to notify user", "sendErr", sendErr)
		}
		return
	}

	ext := utils.ExtensionFor(audioMsg.GetMimetype())
	path := filepath.Join(audioDir, utils.SanitizeJID(sender.String())+ext)
	if err := os.WriteFile(path, data, 0644); err != nil {
		slog.Error("File save failed", "err", err)
		if sendErr := sendTextRaw(ctx, client, evt.Info.Chat, fmt.Sprintf("failed to save audio: %v", err)); sendErr != nil {
			slog.Error("failed to notify user", "sendErr", sendErr)
		}
		return
	}

	// Transcode to MP3 via ffmpeg for playback
	path, err = utils.TranscodeToMP3(path)
	if err != nil {
		slog.Error("Transcode failed", "err", err)
		if sendErr := sendTextRaw(ctx, client, evt.Info.Chat, fmt.Sprintf("failed to process audio: %v", err)); sendErr != nil {
			slog.Error("failed to notify user", "sendErr", sendErr)
		}
		return
	}

	if saveRequested {
		if err := saveAudio(cctx, sender, path); err != nil {
			slog.Error("saveAudio failed", "err", err)
			logHandlerErr("call-audio-save", err)
		}
	}

	slog.Debug("Triggering outgoing call to target", "target", p.Target, "media", path)
	if err := placeCallWithAudio(cctx, p.Target, path); err != nil {
		slog.Error("placeCallWithAudio failed", "err", err)
		logHandlerErr("call", err)
	}
}

func handleVideoDownload(ctx context.Context, client *whatsmeow.Client, cctx *Context, sender types.JID, evt *events.Message, videoMsg *waE2E.VideoMessage, p *pendingCall, saveRequested bool) {
	slog.Debug("Downloading video payload", "sender", sender.String())
	data, err := client.Download(ctx, videoMsg)
	if err != nil {
		slog.Error("Download video failed", "err", err)
		if sendErr := sendTextRaw(ctx, client, evt.Info.Chat, fmt.Sprintf("failed to download video: %v", err)); sendErr != nil {
			slog.Error("failed to notify user", "sendErr", sendErr)
		}
		return
	}

	if err := os.MkdirAll(videoDir, 0755); err != nil {
		slog.Error("Failed creating video directory", "err", err)
		if sendErr := sendTextRaw(ctx, client, evt.Info.Chat, fmt.Sprintf("failed to prepare storage: %v", err)); sendErr != nil {
			slog.Error("failed to notify user", "sendErr", sendErr)
		}
		return
	}

	ext := utils.ExtensionFor(videoMsg.GetMimetype())
	if ext == "" {
		ext = ".mp4"
	}
	path := filepath.Join(videoDir, utils.SanitizeJID(sender.String())+ext)
	if err := os.WriteFile(path, data, 0644); err != nil {
		slog.Error("File save failed", "err", err)
		if sendErr := sendTextRaw(ctx, client, evt.Info.Chat, fmt.Sprintf("failed to save video: %v", err)); sendErr != nil {
			slog.Error("failed to notify user", "sendErr", sendErr)
		}
		return
	}

	// Pre-transcode video to WhatsApp-compatible H.264 and MP3 audio
	_, _, _ = utils.PrepareCallVideo(path)

	if saveRequested {
		if err := saveVideo(cctx, sender, path); err != nil {
			slog.Error("saveVideo failed", "err", err)
			logHandlerErr("call-video-save", err)
		}
	}

	slog.Debug("Triggering outgoing video call to target", "target", p.Target, "media", path)
	if err := placeVideoCallWithMedia(cctx, p.Target, path); err != nil {
		slog.Error("placeVideoCallWithMedia failed", "err", err)
		logHandlerErr("videocall", err)
	}
}
