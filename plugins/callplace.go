// Call placement – place voice/video calls to phone numbers.
package commands

import (
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"whatsrook/utils"

	"github.com/purpshell/meowcaller"
	"github.com/rs/zerolog"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

var (
	meowCallerMu     sync.Mutex
	meowCallerClient *meowcaller.Client
	meowCallerWA     *whatsmeow.Client
)

func RegisterMeowCaller(wa *whatsmeow.Client) *meowcaller.Client {
	meowCallerMu.Lock()
	defer meowCallerMu.Unlock()

	logger := zerolog.Nop()
	mc := meowcaller.NewClient(wa, meowcaller.WithLogger(logger))
	meowCallerClient = mc
	meowCallerWA = wa
	return mc
}

func getMeowCallerClient(wa *whatsmeow.Client) *meowcaller.Client {
	meowCallerMu.Lock()
	defer meowCallerMu.Unlock()

	if meowCallerClient != nil && meowCallerWA == wa {
		return meowCallerClient
	}

	return RegisterMeowCaller(wa)
}

// placeCallWithAudio places a call and plays audioPath to the peer once media
// is ready, then hangs up automatically once the audio should have finished.
func placeCallWithAudio(ctx *Context, target, audioPath string) error {
	client := getMeowCallerClient(ctx.Client)

	var targetJID types.JID
	if strings.Contains(target, "@") {
		targetJID, _ = types.ParseJID(target)
	} else {
		targetJID = types.NewJID(target, types.DefaultUserServer)
	}

	userTag, mentionJID := ctx.FormatMention(targetJID)

	call, err := client.Call(ctx.Ctx, target)
	if err != nil {
		return ctx.ReplyWithMentions(fmt.Sprintf("Call to %s failed: %v", userTag, err), []types.JID{mentionJID})
	}

	duration, durErr := utils.AudioDuration(audioPath)
	if durErr != nil {
		logHandlerErr("call", fmt.Errorf("could not determine audio duration, using 30s fallback: %w", durErr))
		duration = 30 * time.Second
	}

	var startOnce sync.Once
	startMedia := func() {
		startOnce.Do(func() {
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
	}

	call.OnPeerAccept(func() {
		startMedia()
	})

	call.OnReady(func() {
		startMedia()
	})

	call.OnEnd(func(reason string) {
		if err := ctx.ReplyWithMentions(fmt.Sprintf("Call with %s ended: %s", userTag, reason), []types.JID{mentionJID}); err != nil {
			logHandlerErr("call", err)
		}
	})

	return ctx.ReplyWithMentions(fmt.Sprintf("Calling %s...", userTag), []types.JID{mentionJID})
}

// placeVideoCall places an outbound video call to target.
func placeVideoCall(ctx *Context, target string) error {
	return placeVideoCallWithMedia(ctx, target, "")
}

// placeVideoCallWithMedia places an outbound video call to target, playing videoPath media if provided.
func placeVideoCallWithMedia(ctx *Context, target, videoPath string) error {
	client := getMeowCallerClient(ctx.Client)

	var targetJID types.JID
	if strings.Contains(target, "@") {
		targetJID, _ = types.ParseJID(target)
	} else {
		targetJID = types.NewJID(target, types.DefaultUserServer)
	}

	userTag, mentionJID := ctx.FormatMention(targetJID)

	call, err := client.CallWithOptions(ctx.Ctx, target, meowcaller.CallOptions{Video: true})
	if err != nil {
		return ctx.ReplyWithMentions(fmt.Sprintf("Video call to %s failed: %v", userTag, err), []types.JID{mentionJID})
	}

	var requestKeyframe atomic.Bool
	requestKeyframe.Store(true) // start at keyframe

	var startOnce sync.Once
	startMedia := func() {
		startOnce.Do(func() {
			log.Printf("[INFO] videocall: starting media playback (state=%v, video_path=%q)", call.State(), videoPath)

			// Send video state enabled stanza
			_ = call.SetVideoEnabled(true)

			if videoPath != "" {
				mp3Path, h264Path, prepErr := utils.PrepareCallVideo(videoPath)
				if prepErr != nil {
					logHandlerErr("videocall", fmt.Errorf("failed to prepare call video: %w", prepErr))
				}
				log.Printf("[INFO] videocall: prep done mp3=%q h264=%q err=%v", mp3Path, h264Path, prepErr)

				duration, durErr := utils.AudioDuration(videoPath)
				if durErr != nil {
					duration = 30 * time.Second
				}
				log.Printf("[INFO] videocall: media duration=%v", duration)

				// 1. Play audio track if available
				audioFile := mp3Path
				if audioFile == "" {
					audioFile = videoPath
				}
				if src, err := openAudioSource(audioFile); err == nil {
					log.Printf("[INFO] videocall: audio source opened, starting playback from %q", audioFile)
					call.Play(src)
				} else {
					log.Printf("[WARN] videocall: could not open audio source %q: %v", audioFile, err)
				}

				// 2. Stream H.264 video frames
				if h264Path == "" {
					log.Printf("[WARN] videocall: no h264 path after prep, skipping video send")
				} else {
					h264Data, readErr := os.ReadFile(h264Path)
					if readErr != nil {
						log.Printf("[WARN] videocall: failed to read h264 file %q: %v", h264Path, readErr)
					} else if len(h264Data) == 0 {
						log.Printf("[WARN] videocall: h264 file is empty: %q", h264Path)
					} else {
						frames := utils.SplitAnnexBAccessUnits(h264Data)
						log.Printf("[INFO] videocall: split h264 into %d access units (%d bytes total)", len(frames), len(h264Data))
						if len(frames) > 0 {
							// Index all IDR keyframe positions in the video stream
							var idrIndices []int
							for i, f := range frames {
								if utils.AnnexBHasIDR(f) {
									idrIndices = append(idrIndices, i)
								}
							}
							log.Printf("[INFO] videocall: found %d IDR keyframe positions out of %d total frames", len(idrIndices), len(frames))

							go func() {
								frameDur := 66 * time.Millisecond // ~15 FPS
								ticker := time.NewTicker(frameDur)
								defer ticker.Stop()

								frameIdx := 0
								sent := 0
								suppressed := 0
								for {
									select {
									case <-ctx.Ctx.Done():
										log.Printf("[INFO] videocall: context cancelled after %d frames sent", sent)
										return
									case <-ticker.C:
										if call.State() == meowcaller.CallPhaseEnded {
											log.Printf("[INFO] videocall: call ended after %d frames sent", sent)
											return
										}

										// If keyframe was requested (or at startup), jump to nearest IDR keyframe
										if requestKeyframe.Swap(false) {
											bestIdx := 0
											for _, idx := range idrIndices {
												if idx >= frameIdx {
													bestIdx = idx
													break
												}
											}
											frameIdx = bestIdx
											log.Printf("[INFO] videocall: keyframe triggered, jumping frameIdx to %d", frameIdx)
										}

										frame := frames[frameIdx]
										if err := call.SendVideoWithDuration(frame, frameDur); err != nil {
											if strings.Contains(err.Error(), "has no active video media") {
												suppressed++
												if suppressed == 1 || suppressed%30 == 0 {
													log.Printf("[WARN] videocall: no active video media (suppressed=%d, frame=%d)", suppressed, frameIdx)
												}
											} else {
												logHandlerErr("videocall", err)
											}
										} else {
											frameIdx = (frameIdx + 1) % len(frames)
											sent++
											if sent == 1 || sent%30 == 0 {
												log.Printf("[INFO] videocall: sent frame #%d (access_unit=%d, bytes=%d)", sent, frameIdx, len(frame))
											}
										}
									}
								}
							}()
						}
					}
				}

				// 3. Auto-hangup timer
				go func() {
					time.Sleep(duration + 2*time.Second)
					log.Printf("[INFO] videocall: auto-hangup firing after %v", duration)
					if hErr := call.Hangup(); hErr != nil {
						logHandlerErr("videocall", hErr)
					}
				}()
			}
		})
	}

	// Force an immediate IDR keyframe when peer accepts or requests keyframe (PLI/FIR)
	call.OnPeerAccept(func() {
		log.Printf("[INFO] videocall: peer accepted, queuing immediate IDR keyframe")
		requestKeyframe.Store(true)
		startMedia()
	})

	call.OnVideoKeyframeRequest(func() {
		log.Printf("[INFO] videocall: keyframe requested by peer PLI/FIR, queuing IDR keyframe")
		requestKeyframe.Store(true)
	})

	call.OnReady(func() {
		log.Printf("[INFO] videocall: media ready (inbound RTP flowing)")
		startMedia()
	})

	call.OnEnd(func(reason string) {
		if err := ctx.ReplyWithMentions(fmt.Sprintf("Video call with %s ended: %s", userTag, reason), []types.JID{mentionJID}); err != nil {
			logHandlerErr("videocall", err)
		}
	})

	return ctx.ReplyWithMentions(fmt.Sprintf("Video calling %s...", userTag), []types.JID{mentionJID})
}

func placeGroupCall(ctx *Context, groupJID string, participants []string, audioPath string) error {
	client := getMeowCallerClient(ctx.Client)

	groupName := "the group"
	if info, err := ctx.Client.GetGroupInfo(ctx.Ctx, ctx.Chat); err == nil && info != nil {
		if info.GroupName.Name != "" {
			groupName = info.GroupName.Name
		} else if info.Name != "" {
			groupName = info.Name
		}
	}

	opts := meowcaller.GroupCallOptions{
		GroupJID: groupJID,
	}

	var call *meowcaller.Call
	var err error
	var mentions []types.JID

	if len(participants) > 0 {
		if len(participants) > 31 {
			participants = participants[:31]
		}
		for _, pStr := range participants {
			var pJID types.JID
			if strings.Contains(pStr, "@") {
				pJID, _ = types.ParseJID(pStr)
			} else {
				pJID = types.NewJID(pStr, types.DefaultUserServer)
			}
			_, mentionJID := ctx.FormatMention(pJID)
			mentions = append(mentions, mentionJID)
		}
		call, err = client.GroupCallWithOptions(ctx.Ctx, participants, opts)
	} else {
		call, err = client.GroupCallByIDWithOptions(ctx.Ctx, groupJID, opts)
	}

	if err != nil {
		if len(mentions) > 0 {
			return ctx.ReplyWithMentions(fmt.Sprintf("Group call in %s failed: %v", groupName, err), mentions)
		}
		return ctx.Reply(fmt.Sprintf("Group call in %s failed: %v", groupName, err))
	}

	if audioPath != "" {
		duration, durErr := utils.AudioDuration(audioPath)
		if durErr != nil {
			duration = 30 * time.Second
		}

		var startOnce sync.Once
		startMedia := func() {
			startOnce.Do(func() {
				src, err := openAudioSource(audioPath)
				if err != nil {
					logHandlerErr("groupcall", err)
					_ = call.Hangup()
					return
				}
				call.Play(src)
				go func() {
					time.Sleep(duration + 2*time.Second)
					_ = call.Hangup()
				}()
			})
		}

		call.OnPeerAccept(func() {
			startMedia()
		})
		call.OnReady(func() {
			startMedia()
		})
	}

	call.OnEnd(func(reason string) {
		endMsg := fmt.Sprintf("Group call in %s ended: %s", groupName, reason)
		if len(mentions) > 0 {
			_ = ctx.ReplyWithMentions(endMsg, mentions)
		} else {
			_ = ctx.Reply(endMsg)
		}
	})

	text := fmt.Sprintf("Initiating group call in %s...", groupName)
	if len(participants) > 0 {
		text = fmt.Sprintf("Initiating group call to %d participants in %s...", len(participants), groupName)
	}

	if len(mentions) > 0 {
		return ctx.ReplyWithMentions(text, mentions)
	}
	return ctx.Reply(text)
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
