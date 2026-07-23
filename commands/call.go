// Call handling – manage incoming/outgoing call audio replies.
package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"whatsrook/store/sqlstore"
	"whatsrook/utils"

	"go.mau.fi/whatsmeow/proto/waE2E"
)

func init() {
	Register(&Command{
		Name:        "call",
		Description: "Call a number, playing your saved (or next-provided) audio",
		Category:    "calls",
		IsPublic:    true,
		Handler:     handleCall,
	})
	Register(&Command{
		Name:        "setcallaudio",
		Description: "Set your default audio file to be played when calling",
		Category:    "calls",
		IsPublic:    true,
		Handler:     handleSetCallAudio,
	})
}

func handleCall(ctx *Context) error {
	targets := ctx.GetTargets()
	if len(targets) < 1 {
		return sendText(ctx, "usage: !call <number>")
	}
	target := targets[0].String()

	if path, ok := getSavedAudio(ctx, ctx.Sender); ok {
		return placeCallWithAudio(ctx, target, path)
	}

	setPending(ctx.Sender, &pendingCall{Target: target, Kind: sqlstore.CallMediaAudio})
	return sendText(ctx, "Reply to an audio file to use for the call.\n"+
		"Reply \"save\" to that audio to make it your default for future calls.")
}

func handleSetCallAudio(ctx *Context) error {
	var audioMsg *waE2E.AudioMessage
	if ext := ctx.Evt.Message.GetExtendedTextMessage(); ext != nil {
		if ci := ext.GetContextInfo(); ci != nil && ci.QuotedMessage != nil {
			audioMsg = ci.QuotedMessage.GetAudioMessage()
		}
	}

	if audioMsg == nil {
		return ctx.Reply("Reply to the audio file you want to set as your default call audio.")
	}

	data, err := ctx.Client.Download(ctx.Ctx, audioMsg)
	if err != nil {
		return ctx.Reply(fmt.Sprintf(" Failed to download audio: %v", err))
	}

	if err := os.MkdirAll("media", 0755); err != nil {
		return ctx.Reply(fmt.Sprintf(" Failed to create media directory: %v", err))
	}

	ext := utils.ExtensionFor(audioMsg.GetMimetype())
	path := filepath.Join("media", utils.SanitizeJID(ctx.Sender.String())+ext)
	if err := os.WriteFile(path, data, 0644); err != nil {
		return ctx.Reply(fmt.Sprintf(" Failed to save audio: %v", err))
	}

	// Transcode to MP3
	path, err = utils.TranscodeToMP3(path)
	if err != nil {
		return ctx.Reply(fmt.Sprintf(" Failed to transcode audio: %v", err))
	}

	if err := saveAudio(ctx, ctx.Sender, path); err != nil {
		return ctx.Reply(fmt.Sprintf(" Failed to save call audio: %v", err))
	}

	return ctx.Reply("Default call audio set successfully.")
}
