// Video call command – register and initiate video calls.
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
		Name:         "videocall",
		Aliases:      []string{"vc", "vcall"},
		Description:  "Place a video call to a target number",
		Category:     "calls",
		HideFromMenu: false,
		IsPublic:     true,
		Handler:      handleVideoCall,
	})
	Register(&Command{
		Name:         "setvideocall",
		Aliases:      []string{"setvc", "setvideocallaudio"},
		Description:  "Set your default video file to be used when video calling",
		Category:     "calls",
		HideFromMenu: false,
		IsPublic:     true,
		Handler:      handleSetVideoCall,
	})
}

func handleVideoCall(ctx *Context) error {
	targets := ctx.GetTargets()
	if len(targets) < 1 {
		return sendText(ctx, "usage: !videocall <number>")
	}
	target := targets[0].String()

	if path, ok := getSavedVideo(ctx, ctx.Sender); ok {
		return placeVideoCallWithMedia(ctx, target, path)
	}

	setPending(ctx.Sender, &pendingCall{Target: target, Kind: sqlstore.CallMediaVideo})
	return sendText(ctx, "Reply to a video file to use for the video call.\n"+
		"Reply \"save\" to that video to make it your default for future video calls.")
}

func handleSetVideoCall(ctx *Context) error {
	var videoMsg *waE2E.VideoMessage
	if msg := ctx.Evt.Message.GetVideoMessage(); msg != nil {
		videoMsg = msg
	} else if ext := ctx.Evt.Message.GetExtendedTextMessage(); ext != nil {
		if ci := ext.GetContextInfo(); ci != nil && ci.QuotedMessage != nil {
			videoMsg = ci.QuotedMessage.GetVideoMessage()
		}
	}

	if videoMsg == nil {
		return ctx.Reply("Reply to the video file you want to set as your default video call video.")
	}

	data, err := ctx.Client.Download(ctx.Ctx, videoMsg)
	if err != nil {
		return ctx.Reply(fmt.Sprintf("Failed to download video: %v", err))
	}

	if err := os.MkdirAll("./media/call-video", 0755); err != nil {
		return ctx.Reply(fmt.Sprintf("Failed to create media directory: %v", err))
	}

	ext := utils.ExtensionFor(videoMsg.GetMimetype())
	if ext == "" {
		ext = ".mp4"
	}
	path := filepath.Join("./media/call-video", utils.SanitizeJID(ctx.Sender.String())+ext)
	if err := os.WriteFile(path, data, 0644); err != nil {
		return ctx.Reply(fmt.Sprintf("Failed to save video: %v", err))
	}

	if err := saveVideo(ctx, ctx.Sender, path); err != nil {
		return ctx.Reply(fmt.Sprintf("Failed to save video call config: %v", err))
	}

	return ctx.Reply("Default video call video set successfully.")
}
