// Play command & YouTube downloader (ytv/yta) – search via Ember API, show interactive quality buttons, then download.
package commands

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"whatsrook/ember"
	"whatsrook/sender"

	"go.mau.fi/whatsmeow"
	waBinary "go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/proto/waE2E"
)

func init() {
	Register(&Command{
		Name:        "play",
		Description: "Search YouTube for a song and download via interactive quality buttons",
		Category:    "media",
		IsPublic:    true,
		Handler:     handlePlay,
	})

	Register(&Command{
		Name:        "ytv",
		Aliases:     []string{"youtubevideo"},
		Description: "Download YouTube video by URL or query with quality selection buttons",
		Category:    "media",
		IsPublic:    true,
		Handler:     handleYTV,
	})

	Register(&Command{
		Name:        "yta",
		Aliases:     []string{"youtubeaudio"},
		Description: "Download YouTube audio by URL or query with quality selection buttons",
		Category:    "media",
		IsPublic:    true,
		Handler:     handleYTA,
	})
}

// handlePlay: audio-focused search. Finds the top result and shows audio quality buttons.
func handlePlay(ctx *Context) error {
	if len(ctx.Args) == 0 {
		return ctx.Reply("Please specify a song name or YouTube URL.")
	}

	// Button response: !play audio|video <videoID>
	firstArg := strings.ToLower(ctx.Args[0])
	if (firstArg == "audio" || firstArg == "video") && len(ctx.Args) >= 2 {
		videoID := ctx.Args[1]
		videoURL := "https://www.youtube.com/watch?v=" + videoID
		return handlePlayDownload(ctx, firstArg, videoURL)
	}

	query := strings.TrimSpace(ctx.RawArgs)
	if query == "" {
		query = strings.Join(ctx.Args, " ")
	}

	videoURL, title, uploader, err := resolveYouTubeURL(ctx, query)
	if err != nil || videoURL == "" {
		return ctx.Reply("Could not find any video for that query.")
	}

	// play = audio mode (music player)
	return sendQualityButtons(ctx, videoURL, title, uploader, "play")
}

// handleYTV: video-focused download with quality selection.
func handleYTV(ctx *Context) error {
	// Button response: !ytv download <videoID>
	if len(ctx.Args) == 2 && strings.ToLower(ctx.Args[0]) == "download" {
		videoURL := "https://www.youtube.com/watch?v=" + ctx.Args[1]
		return handlePlayDownload(ctx, "video", videoURL)
	}

	input := strings.TrimSpace(ctx.RawArgs)
	if input == "" {
		prefix := ctx.GetPrefix()
		return ctx.Reply(fmt.Sprintf("Usage: `%sytv <youtube_url_or_query>`", prefix))
	}

	videoURL, title, uploader, err := resolveYouTubeURL(ctx, input)
	if err != nil || videoURL == "" {
		return ctx.Reply("Could not find any video for that query.")
	}

	return sendQualityButtons(ctx, videoURL, title, uploader, "ytv")
}

// handleYTA: audio-only download with quality selection.
func handleYTA(ctx *Context) error {
	// Button response: !yta download <videoID>
	if len(ctx.Args) == 2 && strings.ToLower(ctx.Args[0]) == "download" {
		videoURL := "https://www.youtube.com/watch?v=" + ctx.Args[1]
		return handlePlayDownload(ctx, "audio", videoURL)
	}

	input := strings.TrimSpace(ctx.RawArgs)
	if input == "" {
		prefix := ctx.GetPrefix()
		return ctx.Reply(fmt.Sprintf("Usage: `%syta <youtube_url_or_query>`", prefix))
	}

	videoURL, title, uploader, err := resolveYouTubeURL(ctx, input)
	if err != nil || videoURL == "" {
		return ctx.Reply("Could not find any video for that query.")
	}

	return sendQualityButtons(ctx, videoURL, title, uploader, "yta")
}

// resolveYouTubeURL resolves a query or a URL to a canonical YouTube watch URL plus metadata.
// If the input is already a URL it is returned as-is with empty title/uploader.
func resolveYouTubeURL(ctx *Context, input string) (videoURL, title, uploader string, err error) {
	if strings.HasPrefix(input, "http://") || strings.HasPrefix(input, "https://") {
		return input, "", "", nil
	}

	results, searchErr := ember.SearchYouTube(ctx.Ctx, input, 1)
	if searchErr != nil {
		slog.Error("resolveYouTubeURL: search failed", "query", input, "err", searchErr)
		return "", "", "", searchErr
	}
	if len(results) == 0 {
		return "", "", "", fmt.Errorf("no results")
	}
	r := results[0]
	return r.URL, r.Title, r.Uploader, nil
}

// extractVideoID returns the "v=" parameter from a YouTube watch URL, or the raw path segment.
func extractVideoID(videoURL string) string {
	// Fast path: standard watch URL
	if idx := strings.Index(videoURL, "watch?v="); idx >= 0 {
		id := videoURL[idx+8:]
		if end := strings.IndexAny(id, "&?#"); end >= 0 {
			id = id[:end]
		}
		return id
	}
	// youtu.be/<id> short links
	if idx := strings.LastIndex(videoURL, "/"); idx >= 0 {
		id := videoURL[idx+1:]
		if qi := strings.IndexByte(id, '?'); qi >= 0 {
			id = id[:qi]
		}
		return id
	}
	return videoURL
}

// sendQualityButtons fetches format info via Ember and sends an interactive message
// with quality-selection buttons. mode is "play" | "ytv" | "yta".
func sendQualityButtons(ctx *Context, videoURL, title, uploader string, mode string) error {
	// Fetch format list from Ember (cookie already registered server-side)
	data, err := ember.Fetch(ctx.Ctx, videoURL, "")
	if err != nil {
		slog.Error("sendQualityButtons: ember.Fetch failed", "url", videoURL, "err", err)
		errStr := strings.ToLower(err.Error())
		if isCookieError2(errStr) {
			return sendCookieHelp(ctx)
		}
		return ctx.Reply(fmt.Sprintf("Failed to fetch video details: %s", err))
	}

	// Use metadata from Fetch if not already populated from search
	if title == "" && data.Title != "" {
		title = data.Title
	}
	if title == "" {
		title = "YouTube Content"
	}
	if uploader == "" && data.Author != "" {
		uploader = data.Author
	}

	duration := formatDuration(data.Duration)
	bodyText := fmt.Sprintf("Title: %s\nChannel: %s\nDuration: %s\n\nAvailable Qualities:", title, uploader, duration)

	prefix := ctx.GetPrefix()
	videoID := extractVideoID(videoURL)

	var buttons []*waE2E.ButtonsMessage_Button

	switch mode {
	case "yta", "play":
		// Audio qualities — the API delivers a single combined stream; present meaningful labels
		audioQualities := []string{"128kbps", "320kbps"}
		cmd := "yta"
		if mode == "play" {
			cmd = "play audio"
		}
		for _, q := range audioQualities {
			btnID := fmt.Sprintf("%s%s %s", prefix, cmd, videoID)
			label := q
			buttons = append(buttons, &waE2E.ButtonsMessage_Button{
				ButtonID:   &btnID,
				ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{DisplayText: &label},
				Type:       waE2E.ButtonsMessage_Button_RESPONSE.Enum(),
			})
		}

	case "ytv":
		// Video — extract available resolutions from formats list
		heights := collectVideoHeights(data)
		if len(heights) == 0 {
			heights = []int{360} // fallback
		}
		for _, h := range heights {
			label := fmt.Sprintf("%dp", h)
			btnID := fmt.Sprintf("%sytv download %s", prefix, videoID)
			buttons = append(buttons, &waE2E.ButtonsMessage_Button{
				ButtonID:   &btnID,
				ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{DisplayText: &label},
				Type:       waE2E.ButtonsMessage_Button_RESPONSE.Enum(),
			})
		}

	default:
		// Generic: one Video button + one Audio button
		topRes := bestVideoResolution(data)
		videoBtnID := fmt.Sprintf("%sytv download %s", prefix, videoID)
		audioBtnID := fmt.Sprintf("%syta download %s", prefix, videoID)
		videoLabel := fmt.Sprintf("Video (%s)", topRes)
		audioLabel := "Audio (128kbps)"
		buttons = []*waE2E.ButtonsMessage_Button{
			{
				ButtonID:   &videoBtnID,
				ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{DisplayText: &videoLabel},
				Type:       waE2E.ButtonsMessage_Button_RESPONSE.Enum(),
			},
			{
				ButtonID:   &audioBtnID,
				ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{DisplayText: &audioLabel},
				Type:       waE2E.ButtonsMessage_Button_RESPONSE.Enum(),
			},
		}
	}

	footer := "Powered by WhatsRook"
	msg := &waE2E.Message{
		DocumentWithCaptionMessage: &waE2E.FutureProofMessage{
			Message: &waE2E.Message{
				ButtonsMessage: &waE2E.ButtonsMessage{
					ContentText: &bodyText,
					FooterText:  &footer,
					HeaderType:  waE2E.ButtonsMessage_EMPTY.Enum(),
					Buttons:     buttons,
				},
			},
		},
	}

	bizNode := waBinary.Node{
		Tag:   "biz",
		Attrs: waBinary.Attrs{},
		Content: []waBinary.Node{
			{
				Tag: "interactive",
				Attrs: waBinary.Attrs{
					"type": "native_flow",
					"v":    "1",
				},
				Content: []waBinary.Node{
					{
						Tag: "native_flow",
						Attrs: waBinary.Attrs{
							"v":    "9",
							"name": "mixed",
						},
					},
				},
			},
		},
	}

	extra := whatsmeow.SendRequestExtra{
		AdditionalNodes: &[]waBinary.Node{bizNode},
	}

	_, err = ctx.Client.SendMessage(ctx.Ctx, ctx.Chat, msg, extra)
	return err
}

// handlePlayDownload performs the actual download and sends the media to WhatsApp.
func handlePlayDownload(ctx *Context, format string, videoURL string) error {
	if !dlLimiter.Acquire(ctx.Sender.String(), 30*time.Second) {
		return ctx.Reply("Please wait a moment before requesting another download.")
	}
	defer dlLimiter.Release(ctx.Sender.String())

	_ = ctx.Reply("Downloading " + format + "...")

	data, err := ember.Fetch(ctx.Ctx, videoURL, "")
	if err != nil {
		slog.Error("handlePlayDownload: ember.Fetch failed", "url", videoURL, "err", err)
		errStr := strings.ToLower(err.Error())
		if isCookieError2(errStr) {
			return sendCookieHelp(ctx)
		}
		return ctx.Reply(fmt.Sprintf("Failed to download: %s", err))
	}

	// For audio mode, filter medias to audio-only
	if format == "audio" {
		for i := range data.Medias {
			if data.Medias[i].IsAudio || data.Medias[i].Type == "audio" {
				data.Medias = []ember.Media{data.Medias[i]}
				break
			}
		}
		// Mark type so sender picks the right path
		data.Type = "audio"
		if len(data.Medias) == 0 && data.URL != "" {
			// Fall back to the top-level URL as audio
			data.Medias = []ember.Media{{URL: data.URL, Type: "audio", Extension: "mp4", IsAudio: true}}
		}
	}

	return sender.SendResult(ctx.Ctx, ctx.Client, ctx.Chat, data)
}

// collectVideoHeights returns unique video heights from formats, sorted ascending (max 3).
func collectVideoHeights(data *ember.Data) []int {
	seen := make(map[int]bool)
	for _, f := range data.Formats {
		if f.Height != nil && *f.Height > 0 {
			// Skip audio-only (no vcodec or vcodec=="none")
			if f.VCodec != nil && *f.VCodec != "none" && *f.VCodec != "" {
				seen[*f.Height] = true
			}
		}
	}
	var list []int
	for h := range seen {
		list = append(list, h)
	}
	// Sort ascending
	for i := 0; i < len(list); i++ {
		for j := i + 1; j < len(list); j++ {
			if list[j] < list[i] {
				list[i], list[j] = list[j], list[i]
			}
		}
	}
	// Cap at 3
	if len(list) > 3 {
		list = list[len(list)-3:]
	}
	return list
}

// bestVideoResolution returns the highest available video resolution label.
func bestVideoResolution(data *ember.Data) string {
	heights := collectVideoHeights(data)
	if len(heights) == 0 {
		return "360p"
	}
	return fmt.Sprintf("%dp", heights[len(heights)-1])
}

// formatDuration converts a duration value (float64 seconds or string) to mm:ss.
func formatDuration(dur any) string {
	switch v := dur.(type) {
	case float64:
		if v <= 0 {
			return "Unknown"
		}
		m := int(v) / 60
		s := int(v) % 60
		return fmt.Sprintf("%d:%02d", m, s)
	case int:
		if v <= 0 {
			return "Unknown"
		}
		return fmt.Sprintf("%d:%02d", v/60, v%60)
	case string:
		return v
	}
	return "Unknown"
}

func isCookieError2(errStr string) bool {
	return strings.Contains(errStr, "sign in to confirm") ||
		strings.Contains(errStr, "use --cookies-from-browser") ||
		strings.Contains(errStr, "use --cookies for the authentication") ||
		strings.Contains(errStr, "login_required") ||
		strings.Contains(errStr, "cookie")
}

func sendCookieHelp(ctx *Context) error {
	prefix := ctx.GetPrefix()
	bodyText := fmt.Sprintf(
		"YouTube cookies are required to process this request.\n\n"+
			"Download & install the Cookie Editor browser extension to get your cookies:\nhttps://cookie-editor.com/#download\n\n"+
			"Use `%scookie` for instructions, then use `%ssetcookie YOUTUBE <netscape_cookie>` to save your Netscape cookies.",
		prefix, prefix)
	return ctx.Reply(bodyText)
}
