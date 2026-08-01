// Play command & YouTube downloader (ytv/yta) – search and download YouTube media via Ember API with interactive quality buttons.
package commands

import (
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"time"

	"whatsrook/ember"
	"whatsrook/sender"

	"github.com/lrstanley/go-ytdlp"
	"go.mau.fi/whatsmeow"
	waBinary "go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/proto/waE2E"
)

func init() {
	Register(&Command{
		Name:        "play",
		Description: "Search YouTube for a song/video and download via interactive quality selection buttons",
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

// handlePlay handles searching YouTube or direct downloading when quality choice is triggered
func handlePlay(ctx *Context) error {
	if len(ctx.Args) == 0 {
		return ctx.Reply("Please specify a song name or YouTube URL.")
	}

	firstArg := strings.ToLower(ctx.Args[0])
	if (firstArg == "video" || firstArg == "audio") && len(ctx.Args) >= 2 {
		return handlePlayDownload(ctx, firstArg, strings.Join(ctx.Args[1:], " "))
	}

	query := strings.TrimSpace(ctx.RawArgs)
	if query == "" {
		query = strings.Join(ctx.Args, " ")
	}

	targetURL := query
	if !strings.HasPrefix(query, "http://") && !strings.HasPrefix(query, "https://") {
		resolved, err := searchYouTubeURL(ctx, query)
		if err != nil || resolved == "" {
			return ctx.Reply("Could not find any video for that query.")
		}
		targetURL = resolved
	}

	return sendInteractiveQualityMessage(ctx, targetURL, "play")
}

func handleYTV(ctx *Context) error {
	link := resolveFetchURL(ctx)
	if link == "" && len(ctx.Args) > 0 {
		link = ctx.RawArgs
	}

	if link == "" {
		prefix := ctx.GetPrefix()
		return ctx.Reply(fmt.Sprintf("Usage: `%sytv <youtube_url_or_query>`", prefix))
	}

	if !strings.HasPrefix(link, "http://") && !strings.HasPrefix(link, "https://") {
		resolved, err := searchYouTubeURL(ctx, link)
		if err != nil || resolved == "" {
			return ctx.Reply("Could not find any video for that query.")
		}
		link = resolved
	}

	return sendInteractiveQualityMessage(ctx, link, "ytv")
}

func handleYTA(ctx *Context) error {
	link := resolveFetchURL(ctx)
	if link == "" && len(ctx.Args) > 0 {
		link = ctx.RawArgs
	}

	if link == "" {
		prefix := ctx.GetPrefix()
		return ctx.Reply(fmt.Sprintf("Usage: `%syta <youtube_url_or_query>`", prefix))
	}

	if !strings.HasPrefix(link, "http://") && !strings.HasPrefix(link, "https://") {
		resolved, err := searchYouTubeURL(ctx, link)
		if err != nil || resolved == "" {
			return ctx.Reply("Could not find any video for that query.")
		}
		link = resolved
	}

	return sendInteractiveQualityMessage(ctx, link, "yta")
}

func searchYouTubeURL(ctx *Context, query string) (string, error) {
	cmd := ytdlp.New().
		PrintJSON().
		JsRuntimes("bun").
		SkipDownload().
		FlatPlaylist()

	if cookiePath, cleanupCookie, ok := GetYouTubeCookieFile(ctx); ok {
		defer cleanupCookie()
		cmd.Cookies(cookiePath)
	}

	res, err := cmd.Run(ctx.Ctx, "ytsearch1:"+query)
	if err != nil {
		slog.Error("play/yt search failed", "query", query, "err", err)
		return "", err
	}

	infos, err := res.GetExtractedInfo()
	if err != nil || len(infos) == 0 {
		return "", fmt.Errorf("no results found")
	}

	info := infos[0]
	if len(info.Entries) > 0 && info.Entries[0] != nil {
		info = info.Entries[0]
	}

	if info.WebpageURL != nil && *info.WebpageURL != "" {
		return *info.WebpageURL, nil
	}
	if info.ID != "" {
		return "https://www.youtube.com/watch?v=" + info.ID, nil
	}
	return "", fmt.Errorf("video url missing")
}

func sendInteractiveQualityMessage(ctx *Context, targetURL string, mode string) error {
	cookie := getYouTubeCookie(ctx)
	data, err := ember.Fetch(ctx.Ctx, targetURL, cookie)
	if err != nil {
		slog.Error("sendInteractiveQualityMessage: ember.Fetch failed", "url", targetURL, "err", err)
		errStr := strings.ToLower(err.Error())
		if strings.Contains(errStr, "cookie") || strings.Contains(errStr, "login") || strings.Contains(errStr, "sign in") {
			return sendCookieHelp(ctx)
		}
		return ctx.Reply(fmt.Sprintf("Failed to fetch video details: %s", err))
	}

	title := data.Title
	if title == "" {
		title = "YouTube Content"
	}
	uploader := data.Author
	durationStr := "Unknown"

	bodyText := fmt.Sprintf("Title: %s\nChannel: %s\nDuration: %s\n\nAvailable Qualities:", title, uploader, durationStr)

	prefix := ctx.GetPrefix()

	var buttons []*waE2E.ButtonsMessage_Button

	if mode == "yta" {
		// Audio qualities
		qualities := []string{"128kbps", "320kbps"}
		for _, q := range qualities {
			btnID := fmt.Sprintf("%splay audio %s", prefix, targetURL)
			buttons = append(buttons, &waE2E.ButtonsMessage_Button{
				ButtonID:   new(btnID),
				ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{DisplayText: new(q)},
				Type:       waE2E.ButtonsMessage_Button_RESPONSE.Enum(),
			})
		}
	} else if mode == "ytv" {
		// Extract available video resolutions
		resMap := make(map[string]bool)
		for _, f := range data.Formats {
			if f.Height != nil && *f.Height > 0 {
				resMap[fmt.Sprintf("%dp", *f.Height)] = true
			} else if resStr, ok := f.Resolution.(string); ok && resStr != "" && resStr != "audio only" {
				if parts := strings.Split(resStr, "x"); len(parts) == 2 {
					if h, err := strconv.Atoi(strings.TrimSpace(parts[1])); err == nil && h > 0 {
						resMap[fmt.Sprintf("%dp", h)] = true
					}
				}
			}
		}

		var resList []int
		for res := range resMap {
			var h int
			if _, err := fmt.Sscanf(res, "%dp", &h); err == nil {
				resList = append(resList, h)
			}
		}
		sort.Ints(resList)

		if len(resList) == 0 {
			resList = []int{360, 720, 1080}
		}

		// Take up to top 3 resolutions for buttons
		if len(resList) > 3 {
			resList = resList[len(resList)-3:]
		}

		for _, h := range resList {
			label := fmt.Sprintf("%dp", h)
			btnID := fmt.Sprintf("%splay video %s", prefix, targetURL)
			buttons = append(buttons, &waE2E.ButtonsMessage_Button{
				ButtonID:   new(btnID),
				ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{DisplayText: new(label)},
				Type:       waE2E.ButtonsMessage_Button_RESPONSE.Enum(),
			})
		}
	} else {
		// Default play mode: Video & Audio buttons with quality labels
		resMap := make(map[string]bool)
		for _, f := range data.Formats {
			if f.Height != nil && *f.Height > 0 {
				resMap[fmt.Sprintf("%dp", *f.Height)] = true
			}
		}
		topRes := "720p"
		if len(resMap) > 0 {
			var maxH int
			for r := range resMap {
				var h int
				if _, err := fmt.Sscanf(r, "%dp", &h); err == nil && h > maxH {
					maxH = h
				}
			}
			if maxH > 0 {
				topRes = fmt.Sprintf("%dp", maxH)
			}
		}

		videoBtnID := fmt.Sprintf("%splay video %s", prefix, targetURL)
		audioBtnID := fmt.Sprintf("%splay audio %s", prefix, targetURL)

		buttons = []*waE2E.ButtonsMessage_Button{
			{
				ButtonID:   new(videoBtnID),
				ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{DisplayText: new("Video (" + topRes + ")")},
				Type:       waE2E.ButtonsMessage_Button_RESPONSE.Enum(),
			},
			{
				ButtonID:   new(audioBtnID),
				ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{DisplayText: new("Audio (128kbps)")},
				Type:       waE2E.ButtonsMessage_Button_RESPONSE.Enum(),
			},
		}
	}

	msg := &waE2E.Message{
		DocumentWithCaptionMessage: &waE2E.FutureProofMessage{
			Message: &waE2E.Message{
				ButtonsMessage: &waE2E.ButtonsMessage{
					ContentText: &bodyText,
					FooterText:  new("Powered by WhatsRook"),
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

func isCookieError(err error) bool {
	if err == nil {
		return false
	}
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "sign in to confirm") ||
		strings.Contains(errStr, "use --cookies-from-browser") ||
		strings.Contains(errStr, "use --cookies for the authentication") ||
		strings.Contains(errStr, "login_required") ||
		strings.Contains(errStr, "cookie")
}

func sendCookieHelp(ctx *Context) error {
	prefix := ctx.GetPrefix()
	bodyText := fmt.Sprintf("YouTube cookies are required to process this request.\n\nDownload & install the Cookie Editor browser extension to get your cookies:\nhttps://cookie-editor.com/#download\n\nUse `%scookie` for instructions, then use `%ssetcookie YOUTUBE <netscape_cookie>` to save your Netscape cookies.", prefix, prefix)
	return ctx.Reply(bodyText)
}

func handlePlayDownload(ctx *Context, format string, target string) error {
	if !dlLimiter.Acquire(ctx.Sender.String(), 30*time.Second) {
		return ctx.Reply("Please wait a moment before requesting another download.")
	}
	defer dlLimiter.Release(ctx.Sender.String())

	targetURL := target
	if !strings.HasPrefix(target, "http://") && !strings.HasPrefix(target, "https://") {
		targetURL = "https://www.youtube.com/watch?v=" + target
	}

	_ = ctx.Reply("Downloading " + format + "...")

	cookie := getYouTubeCookie(ctx)
	data, err := ember.Fetch(ctx.Ctx, targetURL, cookie)
	if err != nil {
		slog.Error("handlePlayDownload: ember.Fetch failed", "url", targetURL, "err", err)
		errStr := strings.ToLower(err.Error())
		if strings.Contains(errStr, "cookie") || strings.Contains(errStr, "login") || strings.Contains(errStr, "sign in") {
			return sendCookieHelp(ctx)
		}
		return ctx.Reply(fmt.Sprintf("Failed to download: %s", err))
	}

	if format == "audio" {
		for i := range data.Medias {
			if data.Medias[i].IsAudio || data.Medias[i].Type == "audio" {
				data.Medias = []ember.Media{data.Medias[i]}
				break
			}
		}
	}

	return sender.SendResult(ctx.Ctx, ctx.Client, ctx.Chat, data)
}
