package commands

import (
	"github.com/Thruqe/whatsrook/store/sqlstore"
)

func init() {
	Register(&Command{
		Name:        "call",
		Description: "Call a number, playing your saved (or next-provided) audio",
		Handler:     handleCall,
	})
}

func handleCall(ctx *Context) error {
	if len(ctx.Args) < 1 {
		return sendText(ctx, "usage: !call <number>")
	}
	target := ctx.Args[0]

	if path, ok := getSavedAudio(ctx, ctx.Sender); ok {
		return placeCallWithAudio(ctx, target, path)
	}

	setPending(ctx.Sender, &pendingCall{Target: target, Kind: sqlstore.CallMediaAudio})
	return sendText(ctx, "🎙️ Reply to this message with an audio file to use for the call.\n"+
		"Reply \"save\" to that audio to make it your default for future calls.")
}
