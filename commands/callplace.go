package commands

import (
	"fmt"
	"time"

	"whatsrook/utils"

	"github.com/purpshell/meowcaller"
	"github.com/rs/zerolog"
)

// placeCallWithAudio places a call and plays audioPath to the peer once media
// is ready, then hangs up automatically once the audio should have finished.
func placeCallWithAudio(ctx *Context, target, audioPath string) error {
	logger := zerolog.Nop()
	client := meowcaller.NewClient(ctx.Client, meowcaller.WithLogger(logger))

	call, err := client.Call(ctx.Ctx, target)
	if err != nil {
		return sendText(ctx, fmt.Sprintf("call failed: %v", err))
	}

	duration, durErr := utils.AudioDuration(audioPath)
	if durErr != nil {
		logHandlerErr("call", fmt.Errorf("could not determine audio duration, using 30s fallback: %w", durErr))
		duration = 30 * time.Second
	}

	call.OnReady(func() {
		src, err := openAudioSource(audioPath)
		if err != nil {
			logHandlerErr("call", err)
			if hErr := call.Hangup(); hErr != nil {
				logHandlerErr("call", hErr)
			}
			return
		}
		call.Play(src)

		// Hang up shortly after the audio should have finished playing.
		go func() {
			time.Sleep(duration + 2*time.Second) // small buffer for jitter/relay startup
			if hErr := call.Hangup(); hErr != nil {
				logHandlerErr("call", hErr)
			}
		}()
	})

	call.OnEnd(func(reason string) {
		if err := sendText(ctx, "call ended: "+reason); err != nil {
			logHandlerErr("call", err)
		}
	})

	return sendText(ctx, " calling "+target+"...")
}

// placeVideoCall places an outbound video call to target.
func placeVideoCall(ctx *Context, target string) error {
	logger := zerolog.Nop()
	client := meowcaller.NewClient(ctx.Client, meowcaller.WithLogger(logger))

	call, err := client.CallWithOptions(ctx.Ctx, target, meowcaller.CallOptions{Video: true})
	if err != nil {
		return sendText(ctx, fmt.Sprintf("video call failed: %v", err))
	}

	call.OnReady(func() {
		// Video stream media is active and ready
	})

	call.OnEnd(func(reason string) {
		if err := sendText(ctx, "video call ended: "+reason); err != nil {
			logHandlerErr("videocall", err)
		}
	})

	return sendText(ctx, " video calling "+target+"...")
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
