// Movie command – search movies/shows using Xer Movie API (MovieBox/Nkiri) and display results with interactive buttons.
package commands

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"go.mau.fi/whatsmeow"
	waBinary "go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/proto/waE2E"
)

const movieAPIHost = "https://xer-movie-api-ten.vercel.app"
const maxDirectVideoSize = 200 * 1024 * 1024 // 200MB threshold

type movieSearchItem struct {
	SubjectID       string `json:"subjectId"`
	SubjectType     int    `json:"subjectType"`
	Title           string `json:"title"`
	ReleaseDate     string `json:"releaseDate"`
	Genre           string `json:"genre"`
	IMDBRatingValue string `json:"imdbRatingValue"`
	DetailPath      string `json:"detailPath"`
}

type movieSearchResponse struct {
	Status string `json:"status"`
	Data   struct {
		Items []movieSearchItem `json:"items"`
	} `json:"data"`
}

type movieDetailResponse struct {
	Status string `json:"status"`
	Data   struct {
		Subject struct {
			SubjectID       string `json:"subjectId"`
			Title           string `json:"title"`
			Description     string `json:"description"`
			ReleaseDate     string `json:"releaseDate"`
			Genre           string `json:"genre"`
			IMDBRatingValue string `json:"imdbRatingValue"`
			DetailPath      string `json:"detailPath"`
		} `json:"subject"`
	} `json:"data"`
}

type movieSourceItem struct {
	ID                string `json:"id"`
	Quality           int    `json:"quality"`
	Size              int64  `json:"size,string"`
	Filename          string `json:"filename"`
	DirectURL         string `json:"directUrl"`
	DownloadURL       string `json:"downloadUrl"`
	WorkerDownloadUrl string `json:"workerDownloadUrl"`
}

type movieSourcesResponse struct {
	Status string `json:"status"`
	Data   struct {
		Sources          []movieSourceItem `json:"sources"`
		ProcessedSources []movieSourceItem `json:"processedSources"`
	} `json:"data"`
}

func init() {
	Register(&Command{
		Name:        "movie",
		Aliases:     []string{"film", "cinema"},
		Description: "Search for movies or TV series and get details & download links via interactive buttons",
		Category:    "media",
		IsPublic:    true,
		Handler:     handleMovie,
	})
}

func sendInteractiveButtons(ctx *Context, bodyText, footerText string, buttons []struct{ ID, Text string }) error {
	var btnList []*waE2E.ButtonsMessage_Button
	for _, b := range buttons {
		btnList = append(btnList, &waE2E.ButtonsMessage_Button{
			ButtonID:   new(b.ID),
			ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{DisplayText: new(b.Text)},
			Type:       waE2E.ButtonsMessage_Button_RESPONSE.Enum(),
		})
	}

	msg := &waE2E.Message{
		DocumentWithCaptionMessage: &waE2E.FutureProofMessage{
			Message: &waE2E.Message{
				ButtonsMessage: &waE2E.ButtonsMessage{
					ContentText: &bodyText,
					FooterText:  new(footerText),
					HeaderType:  waE2E.ButtonsMessage_EMPTY.Enum(),
					Buttons:     btnList,
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

func handleMovie(ctx *Context) error {
	if len(ctx.Args) == 0 {
		p := ctx.GetPrefix()
		return ctx.Reply(fmt.Sprintf("Usage:\n- %smovie <search query>\n- %smovie info <subjectId> <detailPath>\n- %smovie dl <subjectId> <detailPath>\n\nExamples:\n- %smovie Avatar\n- %smovie Inception", p, p, p, p, p))
	}

	subCmd := strings.ToLower(ctx.Args[0])
	if subCmd == "info" && len(ctx.Args) >= 3 {
		return handleMovieInfo(ctx, ctx.Args[1], ctx.Args[2])
	}
	if subCmd == "dl" && len(ctx.Args) >= 3 {
		return handleMovieDL(ctx, ctx.Args[1], ctx.Args[2])
	}
	if subCmd == "get" && len(ctx.Args) >= 3 {
		return handleMovieGetFile(ctx, ctx.Args[1], ctx.Args[2])
	}

	query := ctx.RawArgs
	if query == "" {
		query = strings.Join(ctx.Args, " ")
	}

	searchURL := fmt.Sprintf("%s/api/search/%s?source=1", movieAPIHost, url.QueryEscape(query))
	req, err := http.NewRequestWithContext(ctx.Ctx, "GET", searchURL, nil)
	if err != nil {
		return ctx.Reply("Failed to create movie search request.")
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		slog.Error("movie search HTTP request failed", "query", query, "err", err)
		return ctx.Reply("Failed to connect to movie database.")
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil || resp.StatusCode != http.StatusOK {
		slog.Error("movie search read failed", "status", resp.StatusCode, "err", err)
		return ctx.Reply("Error fetching movie search results.")
	}

	var searchData movieSearchResponse
	if err := json.Unmarshal(bodyBytes, &searchData); err != nil || len(searchData.Data.Items) == 0 {
		return ctx.Reply(fmt.Sprintf("No movies or TV shows found matching %q.", query))
	}

	item := searchData.Data.Items[0]
	p := ctx.GetPrefix()

	var sb strings.Builder
	sb.WriteString("*Movie Search Result*\n\n")
	fmt.Fprintf(&sb, "*Title:* %s\n", item.Title)
	if item.ReleaseDate != "" {
		fmt.Fprintf(&sb, "*Release:* %s\n", item.ReleaseDate)
	}
	if item.Genre != "" {
		fmt.Fprintf(&sb, "*Genre:* %s\n", item.Genre)
	}
	if item.IMDBRatingValue != "" {
		fmt.Fprintf(&sb, "*IMDb Rating:* %s\n", item.IMDBRatingValue)
	}
	sb.WriteString("\nSelect an action below:")

	buttons := []struct{ ID, Text string }{
		{
			ID:   fmt.Sprintf("%smovie info %s %s", p, item.SubjectID, item.DetailPath),
			Text: "Details",
		},
		{
			ID:   fmt.Sprintf("%smovie dl %s %s", p, item.SubjectID, item.DetailPath),
			Text: "Download Links",
		},
	}

	return sendInteractiveButtons(ctx, sb.String(), "Powered by WhatsRook", buttons)
}

func handleMovieInfo(ctx *Context, subjectID, detailPath string) error {
	infoURL := fmt.Sprintf("%s/api/detail?detailPath=%s&source=1", movieAPIHost, url.QueryEscape(detailPath))
	req, err := http.NewRequestWithContext(ctx.Ctx, "GET", infoURL, nil)
	if err != nil {
		return ctx.Reply("Failed to build movie detail request.")
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		slog.Error("movie detail request failed", "detailPath", detailPath, "err", err)
		return ctx.Reply("Failed to fetch movie details.")
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil || resp.StatusCode != http.StatusOK {
		return ctx.Reply("Error reading movie details.")
	}

	var detailData movieDetailResponse
	if err := json.Unmarshal(bodyBytes, &detailData); err != nil || detailData.Data.Subject.Title == "" {
		return ctx.Reply("Could not parse movie details.")
	}

	sub := detailData.Data.Subject
	p := ctx.GetPrefix()

	var sb strings.Builder
	fmt.Fprintf(&sb, "*%s*\n\n", sub.Title)
	if sub.ReleaseDate != "" {
		fmt.Fprintf(&sb, "*Release Date:* %s\n", sub.ReleaseDate)
	}
	if sub.Genre != "" {
		fmt.Fprintf(&sb, "*Genre:* %s\n", sub.Genre)
	}
	if sub.IMDBRatingValue != "" {
		fmt.Fprintf(&sb, "*Rating:* %s\n", sub.IMDBRatingValue)
	}
	if sub.Description != "" {
		fmt.Fprintf(&sb, "\n*Plot:* %s\n", sub.Description)
	}

	buttons := []struct{ ID, Text string }{
		{
			ID:   fmt.Sprintf("%smovie dl %s %s", p, subjectID, detailPath),
			Text: "Download Links",
		},
	}

	return sendInteractiveButtons(ctx, sb.String(), "Powered by WhatsRook", buttons)
}

func handleMovieDL(ctx *Context, subjectID, detailPath string) error {
	sourcesURL := fmt.Sprintf("%s/api/sources/%s?source=1", movieAPIHost, url.QueryEscape(subjectID))
	req, err := http.NewRequestWithContext(ctx.Ctx, "GET", sourcesURL, nil)
	if err != nil {
		return ctx.Reply("Failed to build movie sources request.")
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		slog.Error("movie sources request failed", "subjectID", subjectID, "err", err)
		return ctx.Reply("Failed to fetch movie download sources.")
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil || resp.StatusCode != http.StatusOK {
		return ctx.Reply("Error reading download sources.")
	}

	var sourcesData movieSourcesResponse
	if err := json.Unmarshal(bodyBytes, &sourcesData); err != nil {
		return ctx.Reply("Could not parse download sources.")
	}

	sources := sourcesData.Data.Sources
	if len(sources) == 0 {
		sources = sourcesData.Data.ProcessedSources
	}
	if len(sources) == 0 {
		return ctx.Reply("No download links available for this title.")
	}

	p := ctx.GetPrefix()
	var buttons []struct{ ID, Text string }

	// Determine recommended item: exactly one item with size > 0 and size <= 200MB.
	// Pick the highest quality item matching <= 200MB.
	recIdx := -1
	for i, src := range sources {
		if src.Size > 0 && src.Size <= maxDirectVideoSize {
			recIdx = i // since sources are usually listed in ascending/descending, pick latest match or first match
		}
	}

	for i, src := range sources {
		qStr := strconv.Itoa(src.Quality) + "p"
		if src.Quality == 0 {
			qStr = "HD"
		}
		var szStr string
		if src.Size > 0 {
			szStr = formatBytes(uint64(src.Size))
		} else {
			szStr = "Unknown size"
		}

		btnText := fmt.Sprintf("%s (%s)", qStr, szStr)
		if i == recIdx {
			btnText += " Recommended"
		}

		btnID := fmt.Sprintf("%smovie get %s %s", p, subjectID, src.ID)
		buttons = append(buttons, struct{ ID, Text string }{
			ID:   btnID,
			Text: btnText,
		})
	}

	// Limit to max 3 buttons supported by WhatsApp ButtonsMessage
	if len(buttons) > 3 {
		buttons = buttons[:3]
	}

	bodyText := "*Download Options*\nChoose a resolution below to download the file directly:"
	return sendInteractiveButtons(ctx, bodyText, "Powered by WhatsRook", buttons)
}

func handleMovieGetFile(ctx *Context, subjectID, sourceID string) error {
	sourcesURL := fmt.Sprintf("%s/api/sources/%s?source=1", movieAPIHost, url.QueryEscape(subjectID))
	req, err := http.NewRequestWithContext(ctx.Ctx, "GET", sourcesURL, nil)
	if err != nil {
		return ctx.Reply("Failed to build movie sources request.")
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		slog.Error("movie get sources request failed", "subjectID", subjectID, "err", err)
		return ctx.Reply("Failed to fetch download source.")
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil || resp.StatusCode != http.StatusOK {
		return ctx.Reply("Error reading download sources.")
	}

	var sourcesData movieSourcesResponse
	if err := json.Unmarshal(bodyBytes, &sourcesData); err != nil {
		return ctx.Reply("Could not parse download source.")
	}

	sources := sourcesData.Data.Sources
	if len(sources) == 0 {
		sources = sourcesData.Data.ProcessedSources
	}

	var selectedSource *movieSourceItem
	for _, s := range sources {
		if s.ID == sourceID {
			selectedSource = &s
			break
		}
	}
	if selectedSource == nil && len(sources) > 0 {
		selectedSource = &sources[0]
	}
	if selectedSource == nil {
		return ctx.Reply("Selected download source not found.")
	}

	downloadTargetURL := selectedSource.WorkerDownloadUrl
	if downloadTargetURL == "" {
		downloadTargetURL = selectedSource.DownloadURL
	}
	if downloadTargetURL == "" {
		downloadTargetURL = selectedSource.DirectURL
	}
	if downloadTargetURL == "" {
		return ctx.Reply("No valid download link found for selected source.")
	}

	_ = ctx.Reply("Downloading movie file, please wait...")

	dlReq, err := http.NewRequestWithContext(ctx.Ctx, "GET", downloadTargetURL, nil)
	if err != nil {
		return ctx.Reply("Failed to initiate file download request.")
	}

	dlClient := &http.Client{Timeout: 15 * time.Minute}
	dlResp, err := dlClient.Do(dlReq)
	if err != nil {
		slog.Error("movie file download failed", "url", downloadTargetURL, "err", err)
		return ctx.Reply("Failed to download movie file.")
	}
	defer dlResp.Body.Close()

	if dlResp.StatusCode != http.StatusOK {
		slog.Error("movie file download non-200 status", "status", dlResp.StatusCode)
		return ctx.Reply("Server returned error when downloading movie file.")
	}

	fileData, err := io.ReadAll(dlResp.Body)
	if err != nil || len(fileData) == 0 {
		slog.Error("movie file read failed", "err", err)
		return ctx.Reply("Failed to read downloaded movie file.")
	}

	filename := selectedSource.Filename
	if filename == "" {
		filename = "movie.mp4"
	}

	fileSize := int64(len(fileData))
	slog.Debug("Downloaded movie file", "filename", filename, "size", fileSize)

	if fileSize > maxDirectVideoSize {
		return ctx.ReplyWithDocument(fileData, "video/mp4", filename, filename)
	}
	return ctx.ReplyWithVideo(fileData, "video/mp4", filename)
}
