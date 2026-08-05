// Movie command – search movies/shows using Xer Movie API (MovieBox/Nkiri) and display results with interactive buttons.
package plugins

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.mau.fi/whatsmeow"
	waBinary "go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
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
	Path            string `json:"path"`
	Source          int    `json:"source"`
}

type movieSearchResponse1 struct {
	Status string `json:"status"`
	Data   struct {
		Items []movieSearchItem `json:"items"`
	} `json:"data"`
}

type movieSource2SearchItem struct {
	Title      string `json:"title"`
	Path       string `json:"path"`
	DetailPath string `json:"detailPath"`
	Source     int    `json:"source"`
}

type movieSearchResponse2 struct {
	Status string `json:"status"`
	Data   struct {
		Items []movieSource2SearchItem `json:"items"`
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
		// Source 2 fields
		Title         string `json:"title"`
		Synopsis      string `json:"synopsis"`
		DownloadItems []struct {
			Text            string `json:"text"`
			IntermediateURL string `json:"intermediateUrl"`
		} `json:"downloadItems"`
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
		Description: "Search for movies or TV series across MovieBox and Nkiri with interactive buttons",
		Category:    "media",
		IsPublic:    true,
		Handler:     handleMovie,
	})
}

func sendInteractiveButtonsWithMentions(ctx *Context, bodyText, footerText string, buttons []struct{ ID, Text string }, jids []types.JID) error {
	var btnList []*waE2E.ButtonsMessage_Button
	for _, b := range buttons {
		btnList = append(btnList, &waE2E.ButtonsMessage_Button{
			ButtonID:   new(b.ID),
			ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{DisplayText: new(b.Text)},
			Type:       waE2E.ButtonsMessage_Button_RESPONSE.Enum(),
		})
	}

	var mentioned []string
	for _, j := range jids {
		if !j.IsEmpty() {
			mentioned = append(mentioned, j.ToNonAD().String())
		}
	}

	var cInfo *waE2E.ContextInfo
	if len(mentioned) > 0 {
		cInfo = &waE2E.ContextInfo{
			MentionedJID: mentioned,
		}
	}

	msg := &waE2E.Message{
		DocumentWithCaptionMessage: &waE2E.FutureProofMessage{
			Message: &waE2E.Message{
				ButtonsMessage: &waE2E.ButtonsMessage{
					ContentText: &bodyText,
					FooterText:  new(footerText),
					HeaderType:  waE2E.ButtonsMessage_EMPTY.Enum(),
					Buttons:     btnList,
					ContextInfo: cInfo,
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

func sendInteractiveButtons(ctx *Context, bodyText, footerText string, buttons []struct{ ID, Text string }) error {
	return sendInteractiveButtonsWithMentions(ctx, bodyText, footerText, buttons, nil)
}

func handleMovie(ctx *Context) error {
	if len(ctx.Args) == 0 {
		p := ctx.GetPrefix()
		return ctx.Reply(fmt.Sprintf("Usage:\n- %smovie <search query>\n\nExamples:\n- %smovie Avatar\n- %smovie Super", p, p, p))
	}

	subCmd := strings.ToLower(ctx.Args[0])
	if idxVal, err := strconv.Atoi(subCmd); err == nil && idxVal >= 1 {
		// User selected a numeric index directly, e.g. .movie 5
		return handleMovieSelectByIndex(ctx, idxVal)
	}
	if subCmd == "select" && len(ctx.Args) >= 4 {
		// Args: select <source> <subjectIdOrPath> <detailPath>
		return handleMovieSelect(ctx, ctx.Args[1], ctx.Args[2], ctx.Args[3])
	}
	if subCmd == "select" && len(ctx.Args) == 2 {
		// Args: select <indexNumber>
		if idxVal, err := strconv.Atoi(ctx.Args[1]); err == nil && idxVal >= 1 {
			return handleMovieSelectByIndex(ctx, idxVal)
		}
	}
	if subCmd == "info" && len(ctx.Args) >= 4 {
		return handleMovieInfo(ctx, ctx.Args[1], ctx.Args[2], ctx.Args[3])
	}
	if subCmd == "dl" && len(ctx.Args) >= 4 {
		return handleMovieDL(ctx, ctx.Args[1], ctx.Args[2], ctx.Args[3])
	}
	if subCmd == "get" && len(ctx.Args) >= 4 {
		return handleMovieGetFile(ctx, ctx.Args[1], ctx.Args[2], ctx.Args[3])
	}
	if subCmd == "page" && len(ctx.Args) >= 3 {
		// Args: page <pageNumber> <escapedQuery>
		pageNum, _ := strconv.Atoi(ctx.Args[1])
		if pageNum < 1 {
			pageNum = 1
		}
		query, _ := url.QueryUnescape(strings.Join(ctx.Args[2:], " "))
		return renderMovieSearchResults(ctx, query, pageNum)
	}

	query := ctx.RawArgs
	if query == "" {
		query = strings.Join(ctx.Args, " ")
	}

	return renderMovieSearchResults(ctx, query, 1)
}

type unifiedMovieResult struct {
	Title      string
	Year       string
	Source     int
	SubjectID  string
	DetailPath string
}

var (
	lastSearchMutex   sync.Mutex
	lastSearchResults = make(map[string][]unifiedMovieResult)
)

func handleMovieSelectByIndex(ctx *Context, idxVal int) error {
	lastSearchMutex.Lock()
	results, ok := lastSearchResults[ctx.Chat.String()]
	lastSearchMutex.Unlock()

	if !ok || len(results) == 0 {
		return ctx.Reply("No recent movie search found for this chat. Please perform a search first using `.movie <query>`.")
	}

	if idxVal < 1 || idxVal > len(results) {
		return ctx.Reply(fmt.Sprintf("Invalid selection index %d. Please choose a number between 1 and %d.", idxVal, len(results)))
	}

	item := results[idxVal-1]
	sourceStr := strconv.Itoa(item.Source)
	return handleMovieSelect(ctx, sourceStr, item.SubjectID, item.DetailPath)
}

func renderMovieSearchResults(ctx *Context, query string, page int) error {
	loader := ctx.StartLoader("Searching movies")
	defer loader.Delete()

	client := &http.Client{Timeout: 15 * time.Second}

	// Fetch Source 1 results
	var results []unifiedMovieResult
	s1URL := fmt.Sprintf("%s/api/search/%s?source=1", movieAPIHost, url.QueryEscape(query))
	req1, err := http.NewRequestWithContext(ctx.Ctx, "GET", s1URL, nil)
	if err == nil {
		if resp1, err := client.Do(req1); err == nil {
			body1, _ := io.ReadAll(resp1.Body)
			_ = resp1.Body.Close()
			var res1 movieSearchResponse1
			if json.Unmarshal(body1, &res1) == nil {
				for _, item := range res1.Data.Items {
					year := ""
					if len(item.ReleaseDate) >= 4 {
						year = item.ReleaseDate[:4]
					}
					dPath := item.DetailPath
					if dPath == "" {
						dPath = item.Path
					}
					sID := item.SubjectID
					if sID == "" {
						sID = dPath
					}
					results = append(results, unifiedMovieResult{
						Title:      item.Title,
						Year:       year,
						Source:     1,
						SubjectID:  sID,
						DetailPath: dPath,
					})
				}
			}
		}
	}

	// Fetch Source 2 results (Nkiri)
	s2URL := fmt.Sprintf("%s/api/search/%s?source=2", movieAPIHost, url.QueryEscape(query))
	req2, err := http.NewRequestWithContext(ctx.Ctx, "GET", s2URL, nil)
	if err == nil {
		if resp2, err := client.Do(req2); err == nil {
			body2, _ := io.ReadAll(resp2.Body)
			_ = resp2.Body.Close()
			var res2 movieSearchResponse2
			if json.Unmarshal(body2, &res2) == nil {
				for _, item := range res2.Data.Items {
					dPath := item.DetailPath
					if dPath == "" {
						dPath = item.Path
					}
					results = append(results, unifiedMovieResult{
						Title:      item.Title,
						Year:       "",
						Source:     2,
						SubjectID:  dPath,
						DetailPath: dPath,
					})
				}
			}
		}
	}

	if len(results) == 0 {
		return ctx.Reply(fmt.Sprintf("No movies or TV shows found matching %q across Source 1 and Source 2.", query))
	}

	// Cache full results for this chat
	lastSearchMutex.Lock()
	lastSearchResults[ctx.Chat.String()] = results
	lastSearchMutex.Unlock()

	pageSize := 3
	totalPages := (len(results) + pageSize - 1) / pageSize
	if page < 1 {
		page = 1
	}
	if page > totalPages {
		page = totalPages
	}

	startIdx := (page - 1) * pageSize
	endIdx := startIdx + pageSize
	if endIdx > len(results) {
		endIdx = len(results)
	}

	pageItems := results[startIdx:endIdx]

	p := ctx.GetPrefix()
	var sb strings.Builder
	fmt.Fprintf(&sb, "*Movie Search Results* (Page %d of %d, Total: %d)\n\n", page, totalPages, len(results))

	for idx, item := range pageItems {
		globalIdx := startIdx + idx + 1
		fmt.Fprintf(&sb, "%d. *%s*", globalIdx, item.Title)
		if item.Year != "" {
			fmt.Fprintf(&sb, " (%s)", item.Year)
		}
		fmt.Fprintf(&sb, " [Source %d]\n", item.Source)
	}

	var buttons []struct{ ID, Text string }
	for idx, item := range pageItems {
		globalIdx := startIdx + idx + 1
		btnText := item.Title
		if item.Year != "" {
			btnText = fmt.Sprintf("%d. %s (%s)", globalIdx, item.Title, item.Year)
		} else {
			btnText = fmt.Sprintf("%d. %s", globalIdx, item.Title)
		}
		if len(btnText) > 20 {
			btnText = btnText[:20]
		}

		buttons = append(buttons, struct{ ID, Text string }{
			ID:   fmt.Sprintf("%smovie select %d %s %s", p, item.Source, url.QueryEscape(item.SubjectID), url.QueryEscape(item.DetailPath)),
			Text: btnText,
		})
	}

	// Add 4th button as Next button if there are more pages
	if page < totalPages {
		nextPage := page + 1
		buttons = append(buttons, struct{ ID, Text string }{
			ID:   fmt.Sprintf("%smovie page %d %s", p, nextPage, url.QueryEscape(query)),
			Text: fmt.Sprintf("Next (Page %d)", nextPage),
		})
	} else if page > 1 {
		// If on last page, offer 4th button as Back to Page 1
		buttons = append(buttons, struct{ ID, Text string }{
			ID:   fmt.Sprintf("%smovie page 1 %s", p, url.QueryEscape(query)),
			Text: "First Page",
		})
	}

	sb.WriteString("\nTo select a result, tap a button above or type:\n")
	fmt.Fprintf(&sb, "`%smovie <number>` (e.g. `%smovie 4`)", p, p)

	return sendInteractiveButtons(ctx, sb.String(), fmt.Sprintf("Powered by %s", ctx.GetBotName()), buttons)
}

func handleMovieSelect(ctx *Context, sourceStr, subjectID, detailPath string) error {
	loader := ctx.StartLoader("Fetching movie details...")
	defer loader.Delete()

	unescapedDetailPath, _ := url.QueryUnescape(detailPath)
	unescapedSubjectID, _ := url.QueryUnescape(subjectID)

	infoURL := fmt.Sprintf("%s/api/detail?detailPath=%s&source=%s", movieAPIHost, url.QueryEscape(unescapedDetailPath), sourceStr)
	req, err := http.NewRequestWithContext(ctx.Ctx, "GET", infoURL, nil)
	if err != nil {
		return ctx.Reply("Failed to build movie detail request.")
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		slog.Error("movie detail request failed", "detailPath", unescapedDetailPath, "source", sourceStr, "err", err)
		return ctx.Reply("Failed to fetch movie details.")
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil || resp.StatusCode != http.StatusOK {
		return ctx.Reply("Error reading movie details.")
	}

	var detailData movieDetailResponse
	if err := json.Unmarshal(bodyBytes, &detailData); err != nil {
		return ctx.Reply("Could not parse movie details.")
	}

	subTitle := detailData.Data.Subject.Title
	relDate := detailData.Data.Subject.ReleaseDate
	genre := detailData.Data.Subject.Genre
	rating := detailData.Data.Subject.IMDBRatingValue
	desc := detailData.Data.Subject.Description

	if subTitle == "" {
		subTitle = detailData.Data.Title
	}
	if desc == "" {
		desc = detailData.Data.Synopsis
	}

	p := ctx.GetPrefix()
	var sb strings.Builder
	fmt.Fprintf(&sb, "*%s*\n\n", subTitle)
	if relDate != "" {
		fmt.Fprintf(&sb, "*Release Date:* %s\n", relDate)
	}
	if genre != "" {
		fmt.Fprintf(&sb, "*Genre:* %s\n", genre)
	}
	if rating != "" {
		fmt.Fprintf(&sb, "*Rating:* %s\n", rating)
	}
	if desc != "" {
		fmt.Fprintf(&sb, "\n*Plot:* %s\n", desc)
	}
	sb.WriteString("\nSelect an action below:")

	buttons := []struct{ ID, Text string }{
		{
			ID:   fmt.Sprintf("%smovie info %s %s %s", p, sourceStr, url.QueryEscape(unescapedSubjectID), url.QueryEscape(unescapedDetailPath)),
			Text: "Details",
		},
		{
			ID:   fmt.Sprintf("%smovie dl %s %s %s", p, sourceStr, url.QueryEscape(unescapedSubjectID), url.QueryEscape(unescapedDetailPath)),
			Text: "Download Links",
		},
	}

	return sendInteractiveButtons(ctx, sb.String(), fmt.Sprintf("Powered by %s", ctx.GetBotName()), buttons)
}

func handleMovieInfo(ctx *Context, sourceStr, subjectID, detailPath string) error {
	return handleMovieSelect(ctx, sourceStr, subjectID, detailPath)
}

func handleMovieDL(ctx *Context, sourceStr, subjectID, detailPath string) error {
	unescapedDetailPath, _ := url.QueryUnescape(detailPath)
	unescapedSubjectID, _ := url.QueryUnescape(subjectID)

	sourcesURL := fmt.Sprintf("%s/api/sources/%s?detailPath=%s&source=%s", movieAPIHost, url.QueryEscape(unescapedSubjectID), url.QueryEscape(unescapedDetailPath), sourceStr)
	req, err := http.NewRequestWithContext(ctx.Ctx, "GET", sourcesURL, nil)
	if err != nil {
		return ctx.Reply("Failed to build movie sources request.")
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		slog.Error("movie sources request failed", "subjectID", unescapedSubjectID, "source", sourceStr, "err", err)
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

	p := ctx.GetPrefix()
	var buttons []struct{ ID, Text string }

	if len(sources) > 0 {
		recIdx := -1
		for i, src := range sources {
			if src.Size > 0 && src.Size <= maxDirectVideoSize {
				recIdx = i
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
				szStr = "Link"
			}

			btnText := fmt.Sprintf("%s (%s)", qStr, szStr)
			if i == recIdx {
				btnText += " Recommended"
			}

			btnID := fmt.Sprintf("%smovie get %s %s %s", p, sourceStr, url.QueryEscape(unescapedSubjectID), url.QueryEscape(src.ID))
			buttons = append(buttons, struct{ ID, Text string }{
				ID:   btnID,
				Text: btnText,
			})
		}

		if len(buttons) > 3 {
			buttons = buttons[:3]
		}

		bodyText := "*Download Options*\nChoose a resolution below to download the file directly:"
		return sendInteractiveButtons(ctx, bodyText, fmt.Sprintf("Powered by %s", ctx.GetBotName()), buttons)
	}

	return ctx.Reply("No direct download links available for this title.")
}

func handleMovieGetFile(ctx *Context, sourceStr, subjectID, sourceID string) error {
	unescapedSubjectID, _ := url.QueryUnescape(subjectID)
	unescapedSourceID, _ := url.QueryUnescape(sourceID)

	sourcesURL := fmt.Sprintf("%s/api/sources/%s?source=%s", movieAPIHost, url.QueryEscape(unescapedSubjectID), sourceStr)
	req, err := http.NewRequestWithContext(ctx.Ctx, "GET", sourcesURL, nil)
	if err != nil {
		return ctx.Reply("Failed to build movie sources request.")
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		slog.Error("movie get sources request failed", "subjectID", unescapedSubjectID, "source", sourceStr, "err", err)
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
		if s.ID == unescapedSourceID {
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
