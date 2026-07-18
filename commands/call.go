package commands

import (
	"github.com/Thruqe/whatsrook/store/sqlstore"
)

func init() {
	Register(&Command{
		Name:        "call",
		Description: "Call a number, playing your saved (or next-provided) audio",
		Category:    "calls",
		IsPublic:    true,
		Handler:     handleCall,
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
