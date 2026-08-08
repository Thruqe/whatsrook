// AutoAcceptCall command & handler – automatically answer incoming calls with pre-set call media.
package plugins

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"whatsrook/utils"
	"whatsrook/wa-core/store/sqlstore"

	"whatsrook/caller"
	"whatsrook/wa-core"
	"whatsrook/wa-core/types/events"
)

const AutoAcceptCallSettingKey = "autoacceptcall_status"

func init() {
	Register(&Command{
		Name:        "autoacceptcall",
		Aliases:     []string{"autoaccept", "acceptcall"},
		Description: "Automatically answer incoming voice and video calls using saved call media",
		Category:    "calls",
		IsPublic:    false,
		Handler:     handleAutoAcceptCallCmd,
	})
}

// ---------------------------------------------------------------------------
// Command handler (user-facing .autoacceptcall on/off/status)
// ---------------------------------------------------------------------------

func handleAutoAcceptCallCmd(ctx *Context) error {
	if !ctx.IsSudo() {
		return ctx.Reply("Only sudoers/bot owners can configure autoacceptcall.")
	}

	s, err := mediaStore(ctx)
	if err != nil {
		return ctx.Reply("Storage unavailable.")
	}

	arg := ""
	if len(ctx.Args) > 0 {
		arg = strings.ToLower(ctx.Args[0])
	}

	switch arg {
	case "on", "enable":
		ownerJID := ctx.Sender.ToNonAD()
		audioPath, errA := s.GetCallMediaConfig(ctx.Ctx, ownerJID, sqlstore.CallMediaAudio)
		videoPath, errV := s.GetCallMediaConfig(ctx.Ctx, ownerJID, sqlstore.CallMediaVideo)

		if errA != nil || audioPath == "" || errV != nil || videoPath == "" {
			var missing []string
			if audioPath == "" {
				missing = append(missing, "- Call Audio (.callaudio <reply to audio>)")
			}
			if videoPath == "" {
				missing = append(missing, "- Call Video (.videocall <reply to video>)")
			}
			p := ctx.GetPrefix()
			return ctx.Reply(fmt.Sprintf("You must configure both call audio and call video media before enabling autoacceptcall.\n\nMissing media:\n%s\n\nConfigure missing media using `%scallaudio` and `%svideocall`.", strings.Join(missing, "\n"), p, p))
		}

		if err := s.PutSetting(ctx.Ctx, AutoAcceptCallSettingKey, "on"); err != nil {
			return ctx.Reply("Failed to enable autoacceptcall.")
		}
		return ctx.Reply("Auto accept call enabled! Incoming voice and video calls will be answered automatically with your saved call media.")

	case "off", "disable":
		if err := s.PutSetting(ctx.Ctx, AutoAcceptCallSettingKey, "off"); err != nil {
			return ctx.Reply("Failed to disable autoacceptcall.")
		}
		return ctx.Reply("Auto accept call disabled.")

	default:
		status, _ := s.GetSetting(ctx.Ctx, AutoAcceptCallSettingKey)
		if status == "" {
			status = "off"
		}
		ownerJID := ctx.Sender.ToNonAD()
		audioPath, _ := s.GetCallMediaConfig(ctx.Ctx, ownerJID, sqlstore.CallMediaAudio)
		videoPath, _ := s.GetCallMediaConfig(ctx.Ctx, ownerJID, sqlstore.CallMediaVideo)

		p := ctx.GetPrefix()
		audioStatus := "Set"
		if audioPath == "" {
			audioStatus = "Not Set"
		}
		videoStatus := "Set"
		if videoPath == "" {
			videoStatus = "Not Set"
		}

		bodyText := fmt.Sprintf("AutoAcceptCall Status: *%s*\n\nRequired Call Media:\n- Call Audio: %s\n- Call Video: %s\n\nUsage:\n- %sautoacceptcall on\n- %sautoacceptcall off", strings.ToUpper(status), audioStatus, videoStatus, p, p)
		return ctx.Reply(bodyText)
	}
}

// ---------------------------------------------------------------------------
// Incoming call handler – set up once in your bot initialization
// ---------------------------------------------------------------------------

// SetupAutoAcceptCall wires the meowcaller OnIncomingCall handler.
// Call this once after creating the meowcaller client.
func SetupAutoAcceptCall(mc *meowcaller.Client, wa *whatsmeow.Client) {
	if mc == nil || wa == nil {
		slog.Error("SetupAutoAcceptCall: nil client")
		return
	}

	mc.OnIncomingCall(func(call *meowcaller.Call) {
		handleIncomingCall(call, wa)
	})
}

func handleIncomingCall(call *meowcaller.Call, waClient *whatsmeow.Client) {
	if call == nil || waClient == nil {
		return
	}

	ctx := context.Background()

	s, ok := waClient.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return
	}

	status, err := s.GetSetting(ctx, AutoAcceptCallSettingKey)
	if err != nil || status != "on" {
		return
	}

	ownerJID := waClient.Store.ID.ToNonAD()
	audioPath, errA := s.GetCallMediaConfig(ctx, ownerJID, sqlstore.CallMediaAudio)
	videoPath, errV := s.GetCallMediaConfig(ctx, ownerJID, sqlstore.CallMediaVideo)

	if errA != nil || audioPath == "" || errV != nil || videoPath == "" {
		slog.Warn("autoacceptcall: enabled but missing required call media", "audio", audioPath, "video", videoPath)
		return
	}

	isVideo := call.IsVideo()
	slog.Info("autoacceptcall: answering incoming call", "from", call.Peer().String(), "call_id", call.ID(), "is_video", isVideo)

	// Set up null receivers BEFORE answering
	call.Receive(meowcaller.SinkFunc(func(pcm []float32) {}))
	if isVideo {
		call.ReceiveVideo(meowcaller.VideoSinkFunc(func(accessUnit []byte) {}))
	}

	// Media starter — sync.Once ensures it runs exactly once
	var mediaOnce sync.Once
	startMedia := func() {
		mediaOnce.Do(func() {
			if isVideo {
				startVideoMedia(call, videoPath)
			} else {
				startAudioMedia(call, audioPath)
			}
		})
	}

	// OnReady fires when first inbound RTP packet arrives
	call.OnReady(func() {
		slog.Info("autoacceptcall: OnReady fired, starting media", "call_id", call.ID())
		startMedia()
	})

	// Let meowcaller handle the full signaling — Answer waits for mute_v2 then sends accept
	if err := call.Answer(); err != nil {
		slog.Error("autoacceptcall: call.Answer() failed", "call_id", call.ID(), "err", err)
		return
	}

	// If OnReady hasn't fired within 10s, something is wrong with the media path.
	// Start anyway — the audio will queue until the relay connects, or fail gracefully.
	go func() {
		time.Sleep(10 * time.Second)
		if call.State() != meowcaller.CallPhaseEnded {
			slog.Info("autoacceptcall: OnReady timeout, starting media anyway", "call_id", call.ID())
			startMedia()
		}
	}()
}

// ---------------------------------------------------------------------------
// Media helpers
// ---------------------------------------------------------------------------

func startAudioMedia(call *meowcaller.Call, audioPath string) {
	slog.Info("autoacceptcall: starting audio media", "call_id", call.ID(), "path", audioPath)

	src, err := meowcaller.MP3File(audioPath)
	if err != nil {
		slog.Error("autoacceptcall: failed to load MP3", "path", audioPath, "err", err)
		_ = call.Hangup()
		return
	}

	call.Play(src)

	duration, err := utils.AudioDuration(audioPath)
	if err != nil {
		duration = 30 * time.Second
	}

	go func() {
		time.Sleep(duration + 2*time.Second)
		if call.State() != meowcaller.CallPhaseEnded {
			slog.Info("autoacceptcall: audio duration exceeded, hanging up", "call_id", call.ID())
			_ = call.Hangup()
		}
	}()
}

func startVideoMedia(call *meowcaller.Call, videoPath string) {
	slog.Info("autoacceptcall: starting video media", "call_id", call.ID(), "path", videoPath)

	mp3Path, h264Path, err := utils.PrepareCallVideo(videoPath)
	if err != nil {
		slog.Error("autoacceptcall: failed to prepare video", "err", err)
		_ = call.Hangup()
		return
	}

	if err := call.SetVideoEnabled(true); err != nil {
		slog.Error("autoacceptcall: SetVideoEnabled failed", "err", err)
	}

	// Play audio track
	audioFile := mp3Path
	if audioFile == "" {
		audioFile = videoPath
	}
	src, err := meowcaller.MP3File(audioFile)
	if err != nil {
		slog.Error("autoacceptcall: failed to load audio", "path", audioFile, "err", err)
		_ = call.Hangup()
		return
	}
	call.Play(src)

	// Send video frames
	if h264Path == "" {
		slog.Warn("autoacceptcall: no h264 track, audio-only for video call", "call_id", call.ID())
		return
	}

	h264Data, err := os.ReadFile(h264Path)
	if err != nil || len(h264Data) == 0 {
		slog.Error("autoacceptcall: failed to read h264", "path", h264Path, "err", err)
		return
	}

	frames := utils.SplitAnnexBAccessUnits(h264Data)
	if len(frames) == 0 {
		slog.Error("autoacceptcall: no video frames", "path", h264Path)
		return
	}

	duration, err := utils.AudioDuration(audioFile)
	if err != nil {
		duration = 30 * time.Second
	}

	go func() {
		ticker := time.NewTicker(66 * time.Millisecond)
		defer ticker.Stop()

		frameIdx := 0
		endTime := time.Now().Add(duration + 2*time.Second)

		for time.Now().Before(endTime) {
			for range ticker.C {
				if call.State() == meowcaller.CallPhaseEnded {
					return
				}
				if err := call.SendVideoWithDuration(frames[frameIdx], 66*time.Millisecond); err != nil {
					slog.Error("autoacceptcall: SendVideoWithDuration failed", "err", err)
					return
				}
				frameIdx = (frameIdx + 1) % len(frames)
			}
		}

		slog.Info("autoacceptcall: video duration exceeded, hanging up", "call_id", call.ID())
		_ = call.Hangup()
	}()
}

// HandleAutoAcceptIncomingCall is the compatibility entry point called from your main bot event handler.
// Since meowcaller handles incoming calls via OnIncomingCall, this is a no-op for the event-based path.
func HandleAutoAcceptIncomingCall(ctx context.Context, client *whatsmeow.Client, v *events.CallOffer) {
	// meowcaller handles incoming calls automatically via OnIncomingCall.
	// This function is kept for compatibility with the existing event dispatcher.
}
