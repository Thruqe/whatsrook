// Call handling – manage incoming/outgoing call audio replies.
package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"whatsrook/store/sqlstore"
	"whatsrook/utils"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
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
	Register(&Command{
		Name:        "groupcall",
		Aliases:     []string{"gcall", "groupvc"},
		Description: "Place a group audio/video call to group members using meowcaller",
		Category:    "calls",
		GroupOnly:   true,
		IsPublic:    true,
		Handler:     handleGroupCall,
	})
}

func handleCall(ctx *Context) error {
	targets := ctx.GetTargets()
	if len(targets) < 1 {
		p := ctx.GetPrefix()
		return ctx.Reply("Usage: " + p + "call <number or reply>")
	}
	target := targets[0].String()

	_ = ctx.Reply("⚠️ Notice: Outgoing call commands are highly unstable on WhatsApp Web protocol and very unlikely to work reliably.")

	if path, ok := getSavedAudio(ctx, ctx.Sender); ok {
		return placeCallWithAudio(ctx, target, path)
	}

	setPending(ctx.Sender, &pendingCall{Target: target, Kind: sqlstore.CallMediaAudio})
	return ctx.Reply("Reply to an audio file to use for the call.\n" +
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
		return ctx.Reply(fmt.Sprintf("Failed to download audio: %v", err))
	}

	if err := os.MkdirAll("media", 0755); err != nil {
		return ctx.Reply(fmt.Sprintf("Failed to create media directory: %v", err))
	}

	ext := utils.ExtensionFor(audioMsg.GetMimetype())
	path := filepath.Join("media", utils.SanitizeJID(ctx.Sender.String())+ext)
	if err := os.WriteFile(path, data, 0644); err != nil {
		return ctx.Reply(fmt.Sprintf("Failed to save audio: %v", err))
	}

	// Transcode to MP3
	path, err = utils.TranscodeToMP3(path)
	if err != nil {
		return ctx.Reply(fmt.Sprintf("Failed to transcode audio: %v", err))
	}

	if err := saveAudio(ctx, ctx.Sender, path); err != nil {
		return ctx.Reply(fmt.Sprintf("Failed to save call audio: %v", err))
	}

	return ctx.Reply("Default call audio set successfully.")
}

func handleGroupCall(ctx *Context) error {
	if ctx.Chat.Server != types.GroupServer {
		return ctx.Reply("The groupcall command can only be used in a group chat.")
	}

	_ = ctx.Reply("⚠️ Notice: Outgoing call commands are highly unstable on WhatsApp Web protocol and very unlikely to work reliably.")

	targets := ctx.GetTargets()
	var participants []string
	if len(targets) > 0 {
		for _, t := range targets {
			participants = append(participants, t.String())
		}
	} else {
		// If no participants specified, fetch group info
		groupInfo, err := ctx.Client.GetGroupInfo(ctx.Ctx, ctx.Chat)
		if err != nil || groupInfo == nil || len(groupInfo.Participants) == 0 {
			return ctx.Reply("Failed to fetch group participants for group call.")
		}
		for _, p := range groupInfo.Participants {
			if !p.JID.IsEmpty() && p.JID.User != ctx.Client.Store.ID.User {
				participants = append(participants, p.JID.String())
			}
		}
	}

	if len(participants) == 0 {
		return ctx.Reply("No valid target participants found for group call.")
	}

	// Limit to max 5 participants for stability
	if len(participants) > 5 {
		participants = participants[:5]
	}

	path, ok := getSavedAudio(ctx, ctx.Sender)
	if !ok {
		path = "" // fallback to default group call without pre-recorded media
	}

	return placeGroupCall(ctx, ctx.Chat.String(), participants, path)
}
