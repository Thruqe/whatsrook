// AutoAcceptCall command & handler – automatically answer incoming calls with pre-set call media.
package commands

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"reflect"
	"strings"
	"sync"
	"time"
	"unsafe"

	"whatsrook/store/sqlstore"
	"whatsrook/utils"

	"github.com/purpshell/meowcaller"
	"github.com/purpshell/meowcaller/signaling"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types/events"
)

const AutoAcceptCallSettingKey = "autoacceptcall_status"

func init() {
	Register(&Command{
		Name:        "autoacceptcall",
		Aliases:     []string{"autoaccept", "acceptcall"},
		Description: "Automatically answer incoming voice and video calls using saved call media",
		Category:    "tools",
		IsPublic:    false,
		Handler:     handleAutoAcceptCallCmd,
	})
}

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
		// Check prerequisites: user MUST have set both audio and video call media
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
			return ctx.Reply(fmt.Sprintf("❌ You must configure both call audio and call video media before enabling autoacceptcall.\n\nMissing media:\n%s\n\nConfigure missing media using `%scallaudio` and `%svideocall`.", strings.Join(missing, "\n"), p, p))
		}

		if err := s.PutSetting(ctx.Ctx, AutoAcceptCallSettingKey, "on"); err != nil {
			return ctx.Reply("Failed to enable autoacceptcall.")
		}
		return ctx.Reply("✅ Auto accept call enabled! Incoming voice and video calls will be answered automatically with your saved call media.")

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
		audioStatus := "✅ Set"
		if audioPath == "" {
			audioStatus = "❌ Not Set"
		}
		videoStatus := "✅ Set"
		if videoPath == "" {
			videoStatus = "❌ Not Set"
		}

		bodyText := fmt.Sprintf("AutoAcceptCall Status: *%s*\n\nRequired Call Media:\n- Call Audio: %s\n- Call Video: %s\n\nUsage:\n- %sautoacceptcall on\n- %sautoacceptcall off", strings.ToUpper(status), audioStatus, videoStatus, p, p)
		return ctx.Reply(bodyText)
	}
}

// HandleIncomingCallAutoAccept handles incoming call offers via meowcaller.Client.OnIncomingCall.
//
// Protocol note: meowcaller defers <accept> until the caller's <mute_v2> stanza arrives.
// However, many WhatsApp callers (especially Android) use stop_probing_before_accept_send=1,
// which means they do NOT send <mute_v2> until AFTER receiving <accept>. This creates a
// deadlock. We break it by sending <accept> directly via DangerousInternals right after
// call.Answer() sets acceptPending, without waiting for <mute_v2>.
func HandleIncomingCallAutoAccept(call *meowcaller.Call) {
	if call == nil || meowCallerWA == nil {
		return
	}

	ctx := context.Background()
	client := meowCallerWA
	s, ok := client.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return
	}

	status, err := s.GetSetting(ctx, AutoAcceptCallSettingKey)
	if err != nil || status != "on" {
		return
	}

	ownerJID := client.Store.ID.ToNonAD()
	audioPath, errA := s.GetCallMediaConfig(ctx, ownerJID, sqlstore.CallMediaAudio)
	videoPath, errV := s.GetCallMediaConfig(ctx, ownerJID, sqlstore.CallMediaVideo)

	if errA != nil || audioPath == "" || errV != nil || videoPath == "" {
		slog.Warn("autoacceptcall: enabled but missing required call media", "audio", audioPath, "video", videoPath)
		return
	}

	slog.Info("autoacceptcall: answering incoming call via meowcaller", "from", call.Peer().String(), "call_id", call.ID(), "is_video", call.IsVideo())

	var startOnce sync.Once
	startMedia := func() {
		startOnce.Do(func() {
			slog.Info("autoacceptcall: media ready, starting playback", "call_id", call.ID(), "is_video", call.IsVideo())

			if call.IsVideo() {
				mp3Path, h264Path, prepErr := utils.PrepareCallVideo(videoPath)
				if prepErr != nil {
					slog.Error("autoacceptcall: failed to prepare video", "err", prepErr)
				}

				duration, durErr := utils.AudioDuration(videoPath)
				if durErr != nil {
					duration = 30 * time.Second
				}

				_ = call.SetVideoEnabled(true)

				audioFile := mp3Path
				if audioFile == "" {
					audioFile = videoPath
				}
				if src, err := openAudioSource(audioFile); err == nil {
					call.Play(src)
				}

				if h264Path != "" {
					if h264Data, rErr := os.ReadFile(h264Path); rErr == nil && len(h264Data) > 0 {
						frames := utils.SplitAnnexBAccessUnits(h264Data)
						if len(frames) > 0 {
							go func() {
								frameDur := 66 * time.Millisecond
								ticker := time.NewTicker(frameDur)
								defer ticker.Stop()

								frameIdx := 0
								endTime := time.Now().Add(duration + 2*time.Second)

								for time.Now().Before(endTime) {
									select {
									case <-ctx.Done():
										_ = call.Hangup()
										return
									case <-ticker.C:
										if call.State() == meowcaller.CallPhaseEnded {
											return
										}
										_ = call.SendVideoWithDuration(frames[frameIdx], frameDur)
										frameIdx = (frameIdx + 1) % len(frames)
									}
								}
								_ = call.Hangup()
							}()
						}
					}
				}
			} else {
				if src, err := openAudioSource(audioPath); err == nil {
					call.Play(src)
					duration, durErr := utils.AudioDuration(audioPath)
					if durErr != nil {
						duration = 30 * time.Second
					}
					go func() {
						time.Sleep(duration + 2*time.Second)
						_ = call.Hangup()
					}()
				} else {
					slog.Error("autoacceptcall: failed to open audio source", "path", audioPath, "err", err)
					_ = call.Hangup()
				}
			}
		})
	}

	// OnReady fires only on the first inbound RTP packet. Since the bot is acting as
	// an answering machine, the caller may wait for us to speak first — so OnReady
	// might never fire. We still register it as a fallback for outbound-initiated flows,
	// but we start media directly in the forced-accept goroutine below.
	call.OnReady(func() {
		startMedia()
	})

	// call.Answer() sets acceptPending=true in meowcaller and calls maybeStartMedia.
	// meowcaller normally waits for <mute_v2> to actually send <accept>, but many
	// Android callers use stop_probing_before_accept_send=1 — they won't send <mute_v2>
	// until they receive <accept> first. Break this deadlock by sending <accept>
	// directly via DangerousInternals after a short relay election delay.
	if err := call.Answer(); err != nil {
		slog.Error("autoacceptcall: failed to answer call", "call_id", call.ID(), "err", err)
		return
	}

	peer := call.Peer()
	callID := call.ID()
	go func() {
		// Wait for relay election to complete (relay latency round-trips take ~500ms–1s).
		// We send <accept> at 1.2s to give relay election time to settle but beat the
		// ~10s caller timeout.
		time.Sleep(1200 * time.Millisecond)

		if call.State() == meowcaller.CallPhaseEnded {
			slog.Debug("autoacceptcall: call already ended before forced accept", "call_id", callID)
			return
		}

		slog.Info("autoacceptcall: sending forced <accept> to break mute_v2 deadlock", "call_id", callID, "peer", peer.String())

		acceptNode := signaling.BuildAccept(&signaling.AcceptParams{
			CallID:      callID,
			To:          peer,
			CallCreator: peer,
			AudioRates:  []string{"16000"},
			Metadata:    map[string]interface{}{"peer_abtest_bucket_id_list": "125208,94276"},
			Video:       call.IsVideo(),
		})
		acceptNode.Attrs["id"] = client.GenerateMessageID()

		if sendErr := client.DangerousInternals().SendNode(context.Background(), acceptNode); sendErr != nil {
			slog.Error("autoacceptcall: failed to send forced accept", "call_id", callID, "err", sendErr)
			return
		}
		slog.Info("autoacceptcall: forced <accept> sent successfully", "call_id", callID)

		// Clear acceptPending in meowcaller's engine so that when the caller responds with mute_v2,
		// meowcaller will NOT send a duplicate <accept> stanza (which causes reconnecting/disconnect).
		clearMeowcallerAcceptPending(call)

		// Give the relay media loop a moment to bind before starting playback.
		// meowcaller's maybeStartMedia was already triggered by answer()+relaylatency,
		// so the media goroutine should be running. We call startMedia here because
		// OnReady fires only on the FIRST inbound RTP — if the caller is waiting for
		// us to speak first, OnReady never fires and we'd never start playing.
		time.Sleep(500 * time.Millisecond)

		if call.State() == meowcaller.CallPhaseEnded {
			slog.Debug("autoacceptcall: call ended before media start", "call_id", callID)
			return
		}
		slog.Info("autoacceptcall: starting media after accept", "call_id", callID)
		startMedia()
	}()
}

// clearMeowcallerAcceptPending clears the internal acceptPending flag on meowcaller.engine
// for the given call. This prevents meowcaller from sending a duplicate <accept> node when
// the incoming mute_v2 arrives.
func clearMeowcallerAcceptPending(call *meowcaller.Call) {
	if call == nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			slog.Warn("autoacceptcall: recover from reflection clearing acceptPending", "err", r)
		}
	}()

	vCall := reflect.ValueOf(call)
	if vCall.Kind() != reflect.Pointer || vCall.IsNil() {
		return
	}
	vCallElem := vCall.Elem()
	fEng := vCallElem.FieldByName("eng")
	if !fEng.IsValid() || fEng.IsNil() {
		return
	}

	engPtr := fEng.Pointer()
	if engPtr == 0 {
		return
	}

	engType := fEng.Type().Elem()
	fMu, hasMu := engType.FieldByName("mu")
	fCalls, hasCalls := engType.FieldByName("calls")
	if !hasMu || !hasCalls {
		return
	}

	mu := (*sync.Mutex)(unsafe.Pointer(engPtr + fMu.Offset))
	mu.Lock()
	defer mu.Unlock()

	callsMapVal := reflect.NewAt(fCalls.Type(), unsafe.Pointer(engPtr+fCalls.Offset)).Elem()
	callID := call.ID()
	mVal := callsMapVal.MapIndex(reflect.ValueOf(callID))
	if !mVal.IsValid() || mVal.IsNil() {
		return
	}

	engineCallPtr := mVal.Pointer()
	if engineCallPtr == 0 {
		return
	}

	engineCallType := mVal.Type().Elem()
	fPending, hasPending := engineCallType.FieldByName("acceptPending")
	if !hasPending {
		return
	}

	pPending := (*bool)(unsafe.Pointer(engineCallPtr + fPending.Offset))
	*pPending = false
	slog.Info("autoacceptcall: cleared acceptPending via reflection to prevent duplicate accept", "call_id", callID)
}

// HandleAutoAcceptIncomingCall is a compatibility helper for CallOffer events.
func HandleAutoAcceptIncomingCall(ctx context.Context, client *whatsmeow.Client, v *events.CallOffer) {
	// meowcaller handles incoming call offers automatically via OnIncomingCall.
}
