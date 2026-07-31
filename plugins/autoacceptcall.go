// AutoAcceptCall command & handler – automatically answer incoming calls with pre-set call media.
package commands

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"whatsrook/store/sqlstore"
	"whatsrook/utils"

	"github.com/purpshell/meowcaller"
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

// HandleAutoAcceptIncomingCall handles an incoming CallOffer when autoacceptcall is ON.
func HandleAutoAcceptIncomingCall(ctx context.Context, client *whatsmeow.Client, v *events.CallOffer) {
	if client == nil || v == nil || client.Store.ID == nil {
		return
	}

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

	slog.Info("autoacceptcall: answering incoming call offer", "from", v.CallCreator.String(), "call_id", v.CallID)

	mc := getMeowCallerClient(client)
	mc.OnIncomingCall(func(call *meowcaller.Call) {
		if call.ID() != v.CallID {
			return
		}

		slog.Info("autoacceptcall: accepting call via meowcaller", "call_id", call.ID())
		if err := call.Answer(); err != nil {
			slog.Error("autoacceptcall: failed to answer call", "err", err)
			return
		}

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
