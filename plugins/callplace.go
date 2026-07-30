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
)

var (
	meowCallerMu     sync.Mutex
	meowCallerClient *meowcaller.Client
	meowCallerWA     *whatsmeow.Client
)

func getMeowCallerClient(wa *whatsmeow.Client) *meowcaller.Client {
	meowCallerMu.Lock()
	defer meowCallerMu.Unlock()

	if meowCallerClient != nil && meowCallerWA == wa {
		return meowCallerClient
	}

	logger := zerolog.Nop()
	meowCallerClient = meowcaller.NewClient(wa, meowcaller.WithLogger(logger))
	meowCallerWA = wa
	return meowCallerClient
}

// placeCallWithAudio places a call and plays audioPath to the peer once media
// is ready, then hangs up automatically once the audio should have finished.
func placeCallWithAudio(ctx *Context, target, audioPath string) error {
	client := getMeowCallerClient(ctx.Client)

	call, err := client.Call(ctx.Ctx, target)
	if err != nil {
		return sendText(ctx, fmt.Sprintf("call failed: %v", err))
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
		if err := sendText(ctx, "call ended: "+reason); err != nil {
			logHandlerErr("call", err)
		}
	})

	return sendText(ctx, " calling "+target+"...")
}

// placeVideoCall places an outbound video call to target.
func placeVideoCall(ctx *Context, target string) error {
	return placeVideoCallWithMedia(ctx, target, "")
}

// placeVideoCallWithMedia places an outbound video call to target, playing videoPath media if provided.
func placeVideoCallWithMedia(ctx *Context, target, videoPath string) error {
	client := getMeowCallerClient(ctx.Client)

	call, err := client.CallWithOptions(ctx.Ctx, target, meowcaller.CallOptions{Video: true})
	if err != nil {
		return sendText(ctx, fmt.Sprintf("video call failed: %v", err))
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
		if err := sendText(ctx, "video call ended: "+reason); err != nil {
			logHandlerErr("videocall", err)
		}
	})

	return sendText(ctx, " video calling "+target+"...")
}

func placeGroupCall(ctx *Context, groupJID string, participants []string, audioPath string) error {
	client := getMeowCallerClient(ctx.Client)

	call, err := client.GroupCall(ctx.Ctx, participants...)
	if err != nil {
		return sendText(ctx, fmt.Sprintf("group call failed: %v", err))
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
		if err := sendText(ctx, "group call ended: "+reason); err != nil {
			logHandlerErr("groupcall", err)
		}
	})

	return sendText(ctx, fmt.Sprintf("initiating group call to %d participants in %s...", len(participants), groupJID))
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
