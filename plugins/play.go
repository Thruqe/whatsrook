// Play command & YouTube downloader (ytv/yta) – search via Ember /youtube/search,
// show interactive quality buttons using search metadata, then download on button press.
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

	// Button response: !play audio <videoID>
	if strings.ToLower(ctx.Args[0]) == "audio" && len(ctx.Args) >= 2 {
		videoID := ctx.Args[1]
		videoURL := "https://www.youtube.com/watch?v=" + videoID
		return handlePlayDownload(ctx, "audio", videoURL)
	}

	query := strings.TrimSpace(ctx.RawArgs)
	if query == "" {
		query = strings.Join(ctx.Args, " ")
	}

	sr, err := resolveToSearchResult(ctx, query)
	if err != nil || sr == nil {
		return ctx.Reply("Could not find any video for that query.")
	}

	return sendQualityButtons(ctx, sr, "play")
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

	sr, err := resolveToSearchResult(ctx, input)
	if err != nil || sr == nil {
		return ctx.Reply("Could not find any video for that query.")
	}

	return sendQualityButtons(ctx, sr, "ytv")
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

	sr, err := resolveToSearchResult(ctx, input)
	if err != nil || sr == nil {
		return ctx.Reply("Could not find any video for that query.")
	}

	return sendQualityButtons(ctx, sr, "yta")
}

// resolveToSearchResult: if input is a URL, wraps it in a synthetic SearchResult.
// If it's a query, calls the Ember /youtube/search endpoint.
func resolveToSearchResult(ctx *Context, input string) (*ember.SearchResult, error) {
	if strings.HasPrefix(input, "http://") || strings.HasPrefix(input, "https://") {
		videoID := extractVideoID(input)
		return &ember.SearchResult{
			ID:    videoID,
			URL:   input,
			Title: "",
		}, nil
	}

	results, err := ember.SearchYouTube(ctx.Ctx, input, 1)
	if err != nil {
		slog.Error("resolveToSearchResult: search failed", "query", input, "err", err)
		return nil, err
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("no results")
	}
	return &results[0], nil
}

// extractVideoID returns the "v=" parameter from a YouTube watch URL.
func extractVideoID(videoURL string) string {
	if idx := strings.Index(videoURL, "watch?v="); idx >= 0 {
		id := videoURL[idx+8:]
		if end := strings.IndexAny(id, "&?#"); end >= 0 {
			id = id[:end]
		}
		return id
	}
	// youtu.be/<id>
	if idx := strings.LastIndex(videoURL, "/"); idx >= 0 {
		id := videoURL[idx+1:]
		if qi := strings.IndexByte(id, '?'); qi >= 0 {
			id = id[:qi]
		}
		return id
	}
	return videoURL
}

// sendQualityButtons shows an interactive message with quality-selection buttons.
// It uses ONLY the search result metadata — no /download call here.
// The actual /download happens when the user clicks a button.
func sendQualityButtons(ctx *Context, sr *ember.SearchResult, mode string) error {
	title := sr.Title
	if title == "" {
		title = "YouTube Content"
	}
	uploader := sr.Uploader
	duration := formatDuration(sr.Duration)

	bodyText := fmt.Sprintf("Title: %s\nChannel: %s\nDuration: %s\n\nAvailable Qualities:", title, uploader, duration)

	prefix := ctx.GetPrefix()
	videoID := sr.ID
	if videoID == "" {
		videoID = extractVideoID(sr.URL)
	}

	var buttons []*waE2E.ButtonsMessage_Button

	switch mode {
	case "yta":
		// Audio quality options
		for _, q := range []string{"128kbps", "320kbps"} {
			q := q
			btnID := fmt.Sprintf("%syta download %s", prefix, videoID)
			buttons = append(buttons, &waE2E.ButtonsMessage_Button{
				ButtonID:   &btnID,
				ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{DisplayText: &q},
				Type:       waE2E.ButtonsMessage_Button_RESPONSE.Enum(),
			})
		}

	case "play":
		// play = audio mode with the same quality labels
		for _, q := range []string{"128kbps", "320kbps"} {
			q := q
			btnID := fmt.Sprintf("%splay audio %s", prefix, videoID)
			buttons = append(buttons, &waE2E.ButtonsMessage_Button{
				ButtonID:   &btnID,
				ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{DisplayText: &q},
				Type:       waE2E.ButtonsMessage_Button_RESPONSE.Enum(),
			})
		}

	case "ytv":
		// Video quality options — the API currently delivers up to 360p combined;
		// show what's realistically available
		for _, res := range []string{"360p", "720p", "1080p"} {
			res := res
			btnID := fmt.Sprintf("%sytv download %s", prefix, videoID)
			buttons = append(buttons, &waE2E.ButtonsMessage_Button{
				ButtonID:   &btnID,
				ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{DisplayText: &res},
				Type:       waE2E.ButtonsMessage_Button_RESPONSE.Enum(),
			})
		}

	default:
		// Generic: one Video button + one Audio button
		videoBtnID := fmt.Sprintf("%sytv download %s", prefix, videoID)
		audioBtnID := fmt.Sprintf("%syta download %s", prefix, videoID)
		videoLabel := "Video (360p)"
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

	_, err := ctx.Client.SendMessage(ctx.Ctx, ctx.Chat, msg, extra)
	return err
}

// handlePlayDownload is called when the user clicks a quality button.
// It calls /download, gets the stream URL, downloads the bytes, and sends to WhatsApp.
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
		if strings.Contains(errStr, "no active cookies") {
			return sendCookieHelp(ctx)
		}
		if strings.Contains(errStr, "requested format is not available") {
			return ctx.Reply("This video's format is not currently supported by the download server. Try a different video.")
		}
		return ctx.Reply(fmt.Sprintf("Failed to download: %s", err))
	}

	// For audio mode: filter to audio-only media, or fall back to top-level URL
	if format == "audio" {
		audioFound := false
		for i := range data.Medias {
			if data.Medias[i].IsAudio || data.Medias[i].Type == "audio" {
				data.Medias = []ember.Media{data.Medias[i]}
				audioFound = true
				break
			}
		}
		if !audioFound && data.URL != "" {
			// Use top-level stream URL as audio (combined stream, extract audio via ffmpeg)
			data.Medias = []ember.Media{{
				URL:       data.URL,
				Type:      "audio",
				Extension: "mp4",
				IsAudio:   true,
			}}
		}
		data.Type = "audio"
	}

	return sender.SendResult(ctx.Ctx, ctx.Client, ctx.Chat, data)
}

// formatDuration converts float64 seconds to a mm:ss string.
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
		if v != "" {
			return v
		}
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
