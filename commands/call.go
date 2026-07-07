package commands

import (
	"log"
)

func init() {
	Register(&Command{
		Name:        "call",
		Description: "Call a number, playing your saved (or next-provided) audio",
		Handler:     handleCall,
	})
}

func handleCall(ctx *Context) error {
	log.Printf("[DEBUG] handleCall triggered. Sender: %s, Args: %v", ctx.Sender.String(), ctx.Args)

	if len(ctx.Args) < 1 {
		log.Printf("[DEBUG] handleCall failed: missing target number argument")
		return sendText(ctx, "usage: !call <number>")
	}
	target := ctx.Args[0]
	log.Printf("[DEBUG] Target phone/JID parsed: %s", target)

	if path, ok := getSavedAudio(ctx, ctx.Sender); ok {
		log.Printf("[DEBUG] Found saved audio for %s at path: %s. Proceeding to place call immediately.", ctx.Sender.String(), path)
		err := placeCallWithAudio(ctx, target, path)
		if err != nil {
			log.Printf("[ERROR] Immediate call placement failed: %v", err)
		}
		return err
	}

	log.Printf("[DEBUG] No saved audio found for %s. Setting state to pendingCall for target: %s", ctx.Sender.String(), target)
	setPending(ctx.Sender, &pendingCall{Target: target})

	// Double-check if your bot framework expects a "." or a "!" prefix!
	log.Printf("[DEBUG] Sending interactive audio prompt reply to chat: %s", ctx.Chat.String())
	return sendText(ctx, "🎙️ Reply to this message with an audio file to use for the call.\n"+
		"Add \"save\" after replying (e.g. reply with caption \"save\") to make it your default for future calls.")
}
