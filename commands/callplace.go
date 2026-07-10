package commands

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/purpshell/meowcaller"
	"github.com/rs/zerolog"
)

func placeCallWithAudio(ctx *Context, target, audioPath string) error {
	log.Printf("[DEBUG] placeCallWithAudio invoked for target: %s, audio: %s", target, audioPath)

	// Use standard debug logging instead of silencing it with Nop() so we can see library faults
	logger := zerolog.New(zerolog.NewConsoleWriter()).Level(zerolog.DebugLevel)
	client := meowcaller.NewClient(ctx.Client, meowcaller.WithLogger(logger))

	// FIX 1: Create a separate background context or a timeout context detached from the
	// transient incoming text message context so it doesn't auto-terminate upon return.
	callCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)

	call, err := client.Call(callCtx, target)
	if err != nil {
		cancel()
		log.Printf("[ERROR] meowcaller client failed to initiate call string: %v", err)
		return sendText(ctx, fmt.Sprintf("call failed: %v", err))
	}
	log.Printf("[DEBUG] Call structure successfully initialized. Call ID: %s", call.ID())

	call.OnReady(func() {
		log.Printf("[DEBUG] Call connected! Media channel ready. Opening source: %s", audioPath)
		src, err := openAudioSource(audioPath)
		if err != nil {
			log.Printf("[ERROR] Failed to open audio source file: %v", err)
			logHandlerErr("call", err)
			call.Hangup()
			return
		}
		log.Printf("[DEBUG] Commencing audio playback pipeline stream for call %s", call.ID())
		call.Play(src)
	})

	call.OnEnd(func(reason string) {
		log.Printf("[DEBUG] Call lifecycle callback OnEnd fired. Reason: %s", reason)
		cancel() // Clean up context resources
		if err := sendText(ctx, "call ended: "+reason); err != nil {
			logHandlerErr("call", err)
		}
	})

	// FIX 2: Spin off a monitor goroutine so meowcaller can keep processing network streams
	// while this function smoothly returns 'nil' to your main dispatcher framework right away.
	go func() {
		defer cancel()
		log.Printf("[DEBUG] Dedicated call monitor goroutine spawned for ID: %s", call.ID())
		// Keep alive while context or call is running
		<-callCtx.Done()
		log.Printf("[DEBUG] Dedicated call monitor goroutine exiting for ID: %s", call.ID())
	}()

	log.Printf("[DEBUG] Sending execution acknowledgment back to chat.")
	return sendText(ctx, "📞 calling "+target+"...")
}

func openAudioSource(path string) (meowcaller.AudioSource, error) {
	switch {
	case hasSuffix(path, ".mp3"):
		return meowcaller.MP3File(path)
	case hasSuffix(path, ".wav"):
		return meowcaller.WAVFile(path)
	case hasSuffix(path, ".opus"), hasSuffix(path, ".ogg"):
		return meowcaller.OpusFile(path)
	default:
		return nil, fmt.Errorf("unsupported audio extension for %s", path)
	}
}

func hasSuffix(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}
