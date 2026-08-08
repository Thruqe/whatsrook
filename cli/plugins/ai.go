// AI command – integrates with Meta AI for question answering and
// command invocation via natural language.
package plugins

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"whatsrook/meta"
	"whatsrook/wa-core/store/sqlstore"

	"whatsrook/wa-core"
	"whatsrook/wa-core/proto/waE2E"
	"whatsrook/wa-core/types"
	"whatsrook/wa-core/types/events"
)

// metaAiBotJID is the fixed JID Meta AI's bot account is reached at.
var metaAiBotJID = types.NewJID("867051314767696", "bot")

type metaAiRequest struct {
	ctx      context.Context
	client   *whatsmeow.Client
	chat     types.JID
	request  string
	onUpdate func(text string) error
	resCh    chan metaAiResponse
}

type metaAiResponse struct {
	res MetaAiResult
	err error
}

var (
	metaAiQueues   = make(map[string]chan metaAiRequest)
	metaAiQueuesMu sync.Mutex
)

func getOrCreateMetaAiQueue(chatKey string) chan metaAiRequest {
	metaAiQueuesMu.Lock()
	defer metaAiQueuesMu.Unlock()
	ch, exists := metaAiQueues[chatKey]
	if !exists {
		ch = make(chan metaAiRequest, 100)
		metaAiQueues[chatKey] = ch
		go func() {
			processMetaAiQueue(ch)
		}()
	}
	return ch
}

func processMetaAiQueue(ch chan metaAiRequest) {
	for req := range ch {
		res, err := executeMetaAiQuery(req.ctx, req.client, req.chat, req.request, req.onUpdate)
		req.resCh <- metaAiResponse{res: res, err: err}
	}
}

// extractMetaAiText pulls the human-readable text out of a Meta AI
// message, regardless of which underlying message shape it used — Meta AI
// has been observed sending plain extendedTextMessage/conversation for
// short replies, and a richer AIRichResponseMessage (with submessages)
// for others.
func extractMetaAiText(msg *waE2E.Message) string {
	if msg == nil {
		return ""
	}
	if conv := msg.GetConversation(); conv != "" {
		return conv
	}
	if ext := msg.GetExtendedTextMessage(); ext != nil {
		return ext.GetText()
	}
	if rich := msg.GetRichResponseMessage(); rich != nil {
		var text strings.Builder
		for _, sub := range rich.GetSubmessages() {
			text.WriteString(sub.GetMessageText())
		}
		return text.String()
	}
	return ""
}

// queryMetaAi sends request to Meta AI's bot JID and streams Meta AI's
// response back to the caller as it arrives.
//
// Meta AI streams its answer by sending an initial placeholder message and
// then repeatedly editing that same message (protocolMessage, type=14,
// key.ID pointing back to its own first message) until a final edit whose
// MsgBotInfo.EditType == "last" arrives. queryMetaAi:
//  1. Sends request as a plain text message to metaAiBotJID.
//  2. Waits for the first incoming message from metaAiBotJID (identified
//     by having no protocolMessage) and captures its own message ID.
//  3. Tracks further edits to that message (matched by the edit's
//     protocolMessage.Key.ID) and calls onUpdate with the latest text on
//     every edit.
//  4. Signals completion once an edit with EditType == "last" arrives,
//     and returns that final text.
//
// Only one in-flight request per chat is allowed at a time; if a request
// for chat is already running, queryMetaAi returns ErrMetaAiBusy
// immediately without sending anything. If ctx is done before a final
// response arrives, queryMetaAi returns ctx.Err().
//
// onUpdate is called synchronously for every partial and the final
// update; pass nil to skip streaming and just get the final text back.
type MetaAiResult struct {
	Text         string
	GeneratedImg []byte
	ImgMimeType  string
	ImgCaption   string
}

type metaAiUnifiedData struct {
	ResponseID string `json:"response_id"`
	Sections   []struct {
		ViewModel struct {
			Primitive struct {
				Typename string `json:"__typename"`
				Media    struct {
					URL      string `json:"url"`
					MimeType string `json:"mime_type"`
				} `json:"media"`
				Status struct {
					Status string `json:"status"`
				} `json:"status"`
				Text string `json:"text"`
			} `json:"primitive"`
		} `json:"view_model"`
	} `json:"sections"`
}

func extractMetaAiGeneratedImage(msg *waE2E.Message) (mediaURL string, mimeType string, text string) {
	if msg == nil {
		return "", "", ""
	}

	var rawB64 []byte
	if rich := msg.GetRichResponseMessage(); rich != nil && rich.GetUnifiedResponse() != nil {
		rawB64 = rich.GetUnifiedResponse().GetData()
	} else if pm := msg.GetProtocolMessage(); pm != nil && pm.GetEditedMessage() != nil {
		if rich := pm.GetEditedMessage().GetRichResponseMessage(); rich != nil && rich.GetUnifiedResponse() != nil {
			rawB64 = rich.GetUnifiedResponse().GetData()
		}
	}

	if len(rawB64) > 0 {
		decoded := make([]byte, base64.StdEncoding.DecodedLen(len(rawB64)))
		n, err := base64.StdEncoding.Decode(decoded, rawB64)
		if err == nil {
			var uData metaAiUnifiedData
			if err := json.Unmarshal(decoded[:n], &uData); err == nil {
				for _, sec := range uData.Sections {
					p := sec.ViewModel.Primitive
					if p.Media.URL != "" {
						mediaURL = strings.ReplaceAll(p.Media.URL, `\/`, `/`)
						mimeType = p.Media.MimeType
					}
					if p.Text != "" {
						text = p.Text
					}
				}
			}
		}
	}
	return mediaURL, mimeType, text
}

// executeMetaAiQuery performs the raw query to Meta AI's bot JID.
func executeMetaAiQuery(ctx context.Context, client *whatsmeow.Client, chat types.JID, request string, onUpdate func(text string) error) (MetaAiResult, error) {
	chatKey := chat.String()

	slog.Debug("executeMetaAiQuery: sending request", "chat", chatKey, "request", request)

	if _, err := client.SendMessage(ctx, metaAiBotJID, &waE2E.Message{
		Conversation: new(request),
	}); err != nil {
		slog.Error("executeMetaAiQuery: failed to send request", "chat", chatKey, "err", err)
		return MetaAiResult{}, fmt.Errorf("failed to send request to meta ai: %w", err)
	}

	var (
		mu         sync.Mutex
		metaMsgID  string
		seen       bool
		finished   bool
		final      string
		genImgData []byte
		genImgMime string
		genImgCap  string
		done       = make(chan struct{})
		closeOnce  sync.Once
	)

	handlerID := client.AddEventHandler(func(evt any) {
		msgEvt, ok := evt.(*events.Message)
		if !ok || msgEvt.Info.Sender.String() != metaAiBotJID.String() {
			return
		}

		pm := msgEvt.Message.GetProtocolMessage()

		mu.Lock()
		if finished {
			if imgMsg := msgEvt.Message.GetImageMessage(); imgMsg != nil {
				slog.Debug("executeMetaAiQuery: captured follow-up imageMessage after finished", "chat", chatKey)
				imgBytes, err := client.Download(ctx, imgMsg)
				if err == nil && len(imgBytes) > 0 {
					genImgData = imgBytes
					genImgMime = imgMsg.GetMimetype()
					if genImgMime == "" {
						genImgMime = "image/jpeg"
					}
					genImgCap = imgMsg.GetCaption()
					slog.Debug("executeMetaAiQuery: successfully downloaded follow-up imageMessage", "len", len(imgBytes))
					closeOnce.Do(func() { close(done) })
				}
			}
			mu.Unlock()
			return
		}

		if imgMsg := msgEvt.Message.GetImageMessage(); imgMsg != nil {
			slog.Debug("executeMetaAiQuery: captured direct imageMessage from Meta AI", "chat", chatKey)
			imgBytes, err := client.Download(ctx, imgMsg)
			if err == nil && len(imgBytes) > 0 {
				genImgData = imgBytes
				genImgMime = imgMsg.GetMimetype()
				if genImgMime == "" {
					genImgMime = "image/jpeg"
				}
				genImgCap = imgMsg.GetCaption()
				slog.Debug("executeMetaAiQuery: successfully downloaded direct imageMessage", "len", len(imgBytes))
				mu.Unlock()
				closeOnce.Do(func() { close(done) })
				return
			}
		}

		if !seen {
			if pm != nil {
				mu.Unlock()
				return
			}
			metaMsgID = msgEvt.Info.ID
			seen = true
			mu.Unlock()
			slog.Debug("executeMetaAiQuery: captured meta ai reply message id", "chat", chatKey, "meta_msg_id", metaMsgID)
		} else if pm == nil || pm.GetKey().GetID() != metaMsgID {
			mu.Unlock()
			return
		} else {
			mu.Unlock()
		}

		// Extract generated image media URL if present
		mediaURL, mimeType, imgCap := extractMetaAiGeneratedImage(msgEvt.Message)
		if mediaURL == "" && pm != nil {
			mediaURL, mimeType, imgCap = extractMetaAiGeneratedImage(pm.GetEditedMessage())
		}
		if mediaURL != "" {
			req, err := http.NewRequestWithContext(ctx, "GET", mediaURL, nil)
			if err == nil {
				req.Header.Set("User-Agent", "WhatsApp/2.24.1.76 A")
				resp, err := http.DefaultClient.Do(req)
				if err == nil && resp.StatusCode == 200 {
					imgBytes, err := io.ReadAll(resp.Body)
					_ = resp.Body.Close()
					if err == nil && len(imgBytes) > 0 {
						mu.Lock()
						genImgData = imgBytes
						genImgMime = mimeType
						genImgCap = imgCap
						mu.Unlock()
						slog.Debug("executeMetaAiQuery: downloaded generated image", "len", len(imgBytes), "mime", mimeType)
					}
				}
			}
		}

		var text string
		if pm == nil {
			text = extractMetaAiText(msgEvt.Message)
		} else {
			text = extractMetaAiText(pm.GetEditedMessage())
		}
		if text == "" {
			slog.Debug("executeMetaAiQuery: empty text extracted, skipping update", "chat", chatKey, "info_id", msgEvt.Info.ID)
			return
		}

		editType := string(msgEvt.Info.MsgBotInfo.EditType)
		slog.Debug("executeMetaAiQuery: update", "chat", chatKey, "edit_type", editType, "text", text)

		if _, _, isRunCmd := meta.ParseRunCommand(text); isRunCmd {
			mu.Lock()
			final = text
			mu.Unlock()
			if editType == "last" || editType == "inner" {
				slog.Debug("executeMetaAiQuery: RUN_COMMAND captured", "chat", chatKey, "cmd_text", text, "edit_type", editType)
				if editType == "last" {
					mu.Lock()
					finished = true
					mu.Unlock()
					closeOnce.Do(func() { close(done) })
					return
				}
			}
		} else if onUpdate != nil {
			if err := onUpdate(text); err != nil {
				slog.Error("executeMetaAiQuery: onUpdate callback failed", "chat", chatKey, "err", err)
			}
		}

		if editType == "last" {
			mu.Lock()
			if final == "" {
				final = text
			}
			finished = true
			hasImgData := len(genImgData) > 0
			mu.Unlock()

			lower := strings.ToLower(text)
			if !hasImgData && (strings.Contains(lower, "image") || strings.Contains(lower, "creating") || strings.Contains(lower, "ready")) {
				slog.Debug("executeMetaAiQuery: text indicates image generation, waiting briefly for follow-up imageMessage", "chat", chatKey)
				go func() {
					time.Sleep(4 * time.Second)
					closeOnce.Do(func() { close(done) })
				}()
			} else {
				closeOnce.Do(func() { close(done) })
			}
		}
	})
	defer client.RemoveEventHandler(handlerID)

	select {
	case <-ctx.Done():
		slog.Warn("executeMetaAiQuery: context cancelled/timed out before completion", "chat", chatKey, "err", ctx.Err())
		return MetaAiResult{}, ctx.Err()
	case <-done:
		mu.Lock()
		defer mu.Unlock()
		slog.Debug("executeMetaAiQuery: completed", "chat", chatKey, "final_text_len", len(final))
		return MetaAiResult{
			Text:         final,
			GeneratedImg: genImgData,
			ImgMimeType:  genImgMime,
			ImgCaption:   genImgCap,
		}, nil
	}
}

// queryMetaAi queues and sends request to Meta AI's bot JID, returning Meta AI's response once available.
func queryMetaAi(ctx context.Context, client *whatsmeow.Client, chat types.JID, request string, onUpdate func(text string) error) (MetaAiResult, error) {
	chatKey := chat.String()
	q := getOrCreateMetaAiQueue(chatKey)

	req := metaAiRequest{
		ctx:      ctx,
		client:   client,
		chat:     chat,
		request:  request,
		onUpdate: onUpdate,
		resCh:    make(chan metaAiResponse, 1),
	}

	select {
	case q <- req:
	case <-ctx.Done():
		return MetaAiResult{}, ctx.Err()
	}

	select {
	case res := <-req.resCh:
		return res.res, res.err
	case <-ctx.Done():
		return MetaAiResult{}, ctx.Err()
	}
}

func init() {
	Register(&Command{
		Name:        "ai",
		Aliases:     []string{"gpt", "ask"},
		Description: "Ask Meta AI a question.",
		Category:    "ai",
		IsPublic:    true,
		Handler:     handleAI,
	})
	Register(&Command{
		Name:        "autoai",
		Description: "Toggle automatic AI responses when tagged, replied to, or when 'Rook' or 'WhatsRook' is mentioned in this chat (on/off)",
		Category:    "ai",
		IsPublic:    true,
		Handler:     handleAutoAI,
	})
	Register(&Command{
		Name:        "csai",
		Aliases:     []string{"customai", "aipersona", "aipersonality"},
		Description: "Configure global AI personality traits and relationship behavior (Sudoers only)",
		Category:    "ai",
		IsPublic:    false,
		Handler:     handleCSAI,
	})
}

func handleAutoAI(ctx *Context) error {
	slog.Debug("handleAutoAI started", "args", ctx.Args)

	isAuthorized := ctx.IsSudo()
	if !isAuthorized && ctx.Chat.Server == "g.us" {
		info, err := ctx.Client.GetGroupInfo(ctx.Ctx, ctx.Chat)
		if err == nil && info != nil {
			if ctx.IsSenderAdmin(info) {
				isAuthorized = true
			}
		}
	}

	if !isAuthorized {
		return ctx.Reply("Only sudoers or group admins can change the AutoAI setting.")
	}

	s, okStore := ctx.Client.Store.Identities.(*sqlstore.SQLStore)
	if !okStore {
		return ctx.Reply("Database store is not available.")
	}

	settingKey := "autoai:" + ctx.Chat.String()

	if len(ctx.Args) == 0 {
		current, _ := s.GetSetting(ctx.Ctx, settingKey)
		if current == "" {
			current = "off"
		}
		return ctx.Reply(fmt.Sprintf("AutoAI is currently %s in this chat.", current))
	}

	val := strings.ToLower(ctx.Args[0])
	if val != "on" && val != "off" {
		return ctx.Reply(fmt.Sprintf("Usage: %sautoai [on/off]", ctx.GetPrefix()))
	}

	if err := s.PutSetting(ctx.Ctx, settingKey, val); err != nil {
		slog.Error("failed to update autoai setting", "err", err)
		return ctx.Reply("Failed to update setting: " + err.Error())
	}

	return ctx.Reply(fmt.Sprintf("AutoAI has been set to %s for this chat.", val))
}

type csaiTrait struct {
	Name        string
	Instruction string
}

var defaultCSAITraits = []csaiTrait{
	{Name: "Professional", Instruction: "Be formal, objective, concise, and highly professional in all responses."},
	{Name: "Friendly & Warm", Instruction: "Be extremely friendly, encouraging, warm, and approachable in tone."},
	{Name: "Sarcastic & Witty", Instruction: "Use playful sarcasm, humor, clever retorts, and witty banter in all interactions."},
	{Name: "Scientific & Precise", Instruction: "Respond with deep technical accuracy, scientific precision, and analytical depth."},
	{Name: "Poetic & Creative", Instruction: "Use eloquent, expressive, poetic, and creative language when answering."},
	{Name: "Motivational Coach", Instruction: "Act as an energetic, inspiring, and relentless motivational coach."},
	{Name: "Pirate", Instruction: "Speak like a pirate using nautical slang, 'Ahoy', 'Matey', and maritime flair."},
	{Name: "Gen-Z & Trendy", Instruction: "Use modern Gen-Z slang, casual expressions, and trendy internet vibe."},
	{Name: "Philosophical Thinker", Instruction: "Reflect deeply on questions, offering thoughtful, philosophical perspectives."},
	{Name: "Strict Sudo Assistant", Instruction: "Treat Sudoers with utmost authority and honor, addressing them respectfully as Master/Boss while serving all requests strictly."},
}

func handleCSAI(ctx *Context) error {
	if !ctx.IsSudo() {
		return ctx.Reply("Only Sudoers can configure global AI personality traits and custom behavior.")
	}

	s, okStore := ctx.Client.Store.Identities.(*sqlstore.SQLStore)
	if !okStore {
		return ctx.Reply("Database store is not available.")
	}

	p := ctx.GetPrefix()

	if len(ctx.Args) >= 2 && strings.ToLower(ctx.Args[0]) == "set" {
		idxVal, err := strconv.Atoi(ctx.Args[1])
		if err == nil && idxVal >= 1 && idxVal <= len(defaultCSAITraits) {
			trait := defaultCSAITraits[idxVal-1]
			if err := s.PutSetting(ctx.Ctx, "csai_prompt", trait.Instruction); err != nil {
				return ctx.Reply("Failed to save AI personality trait.")
			}
			return ctx.Reply(fmt.Sprintf("Saved AI personality trait to *%s*!\n\nInstruction: %s", trait.Name, trait.Instruction))
		}
	}

	if len(ctx.Args) >= 2 && strings.ToLower(ctx.Args[0]) == "custom" {
		customPrompt := strings.TrimSpace(strings.Join(ctx.Args[1:], " "))
		if customPrompt == "" {
			return ctx.Reply(fmt.Sprintf("Usage: `%scsai custom <your prompt / how to refer to you>`\n\nExample: `%scsai custom Always refer to me as Chief and be extremely respectful.`", p, p))
		}
		if err := s.PutSetting(ctx.Ctx, "csai_prompt", customPrompt); err != nil {
			return ctx.Reply("Failed to save custom AI prompt.")
		}
		return ctx.Reply(fmt.Sprintf("Saved custom AI personality prompt!\n\nCustom Prompt: %s", customPrompt))
	}

	if len(ctx.Args) >= 1 && strings.ToLower(ctx.Args[0]) == "reset" {
		_ = s.DeleteSetting(ctx.Ctx, "csai_prompt")
		return ctx.Reply("AI personality prompt has been reset to default.")
	}

	if len(ctx.Args) >= 2 && strings.ToLower(ctx.Args[0]) == "page" {
		pageNum, _ := strconv.Atoi(ctx.Args[1])
		return renderCSAIPage(ctx, s, pageNum)
	}

	// If direct string provided after .csai (e.g. .csai Always call me Boss)
	if len(ctx.Args) > 0 {
		subCmd := strings.ToLower(ctx.Args[0])
		if idxVal, err := strconv.Atoi(subCmd); err == nil && idxVal >= 1 && idxVal <= len(defaultCSAITraits) {
			trait := defaultCSAITraits[idxVal-1]
			_ = s.PutSetting(ctx.Ctx, "csai_prompt", trait.Instruction)
			return ctx.Reply(fmt.Sprintf("Saved AI personality trait to *%s*!\n\nInstruction: %s", trait.Name, trait.Instruction))
		}
		if idxVal, err := strconv.Atoi(subCmd); err == nil && idxVal == 11 {
			return ctx.Reply(fmt.Sprintf("To set a custom trait/prompt, please type:\n`%scsai custom <your custom prompt / how you want the AI to refer to you>`\n\nExample:\n`%scsai custom Always refer to me as Boss and be concise.`", p, p))
		}

		customPrompt := strings.TrimSpace(ctx.RawArgs)
		if err := s.PutSetting(ctx.Ctx, "csai_prompt", customPrompt); err != nil {
			return ctx.Reply("Failed to save custom AI prompt.")
		}
		return ctx.Reply(fmt.Sprintf("Saved custom AI personality prompt!\n\nCustom Prompt: %s", customPrompt))
	}

	return renderCSAIPage(ctx, s, 1)
}

func renderCSAIPage(ctx *Context, s *sqlstore.SQLStore, page int) error {
	currentPrompt, _ := s.GetSetting(ctx.Ctx, "csai_prompt")
	if currentPrompt == "" {
		currentPrompt = "Standard (Default Meta AI behavior)"
	}

	pageSize := 3
	totalPages := (len(defaultCSAITraits) + pageSize - 1) / pageSize
	if page < 1 {
		page = 1
	}
	if page > totalPages {
		page = totalPages
	}

	startIdx := (page - 1) * pageSize
	endIdx := startIdx + pageSize
	if endIdx > len(defaultCSAITraits) {
		endIdx = len(defaultCSAITraits)
	}

	pageItems := defaultCSAITraits[startIdx:endIdx]
	p := ctx.GetPrefix()

	var sb strings.Builder
	fmt.Fprintf(&sb, "*Custom AI Personality & Trait Configuration* (Page %d of %d)\n\n", page, totalPages)
	fmt.Fprintf(&sb, "*Active AI Trait/Prompt:* %s\n\n", currentPrompt)
	sb.WriteString("Select a personality trait for Meta AI below:\n\n")

	for idx, trait := range pageItems {
		globalIdx := startIdx + idx + 1
		fmt.Fprintf(&sb, "%d. *%s*: %s\n", globalIdx, trait.Name, trait.Instruction)
	}
	sb.WriteString("11. *Custom Trait / How You Refer To Me*: Enter your own custom prompt.\n")

	var buttons []struct{ ID, Text string }
	for idx, trait := range pageItems {
		globalIdx := startIdx + idx + 1
		btnText := fmt.Sprintf("%d. %s", globalIdx, trait.Name)
		if len(btnText) > 20 {
			btnText = btnText[:20]
		}
		buttons = append(buttons, struct{ ID, Text string }{
			ID:   fmt.Sprintf("%scsai set %d", p, globalIdx),
			Text: btnText,
		})
	}

	if page < totalPages {
		nextPage := page + 1
		buttons = append(buttons, struct{ ID, Text string }{
			ID:   fmt.Sprintf("%scsai page %d", p, nextPage),
			Text: fmt.Sprintf("Next (Page %d)", nextPage),
		})
	} else {
		// On final page, show 11. Custom Trait button
		buttons = append(buttons, struct{ ID, Text string }{
			ID:   fmt.Sprintf("%scsai custom", p),
			Text: "11. Custom Trait",
		})
	}

	sb.WriteString("\nTo select a personality, tap a button above or type:\n")
	fmt.Fprintf(&sb, "- `%scsai <number>` (e.g. `%scsai 3`)\n", p, p)
	fmt.Fprintf(&sb, "- `%scsai custom <prompt>` (e.g. `%scsai custom Refer to me as Sir`)\n", p, p)
	fmt.Fprintf(&sb, "- `%scsai reset` (to restore default AI behavior)", p)

	return sendInteractiveButtons(ctx, sb.String(), fmt.Sprintf("Powered by %s", ctx.GetBotName()), buttons)
}

func handleAI(ctx *Context) error {
	if len(ctx.Args) == 0 {
		p := ctx.GetPrefix()
		return ctx.Reply(fmt.Sprintf("Usage:\n- %sai <question>\n- %sask <question>\n\nExamples:\n- %sai What is the speed of light?\n- %sask Explain quantum computing in simple terms\n- Reply to an image or message with %sai Analyze this", p, p, p, p, p))
	}

	botName := ctx.GetBotName()
	// Build (or reuse cached) instruction block describing available
	// bot commands.
	instruction := meta.GetOrBuildInstructionWithName(botName, func() string {
		cmdInfos := ListCommands()
		metaCmds := make([]meta.CommandInfo, 0, len(cmdInfos))
		for _, c := range cmdInfos {
			metaCmds = append(metaCmds, meta.CommandInfo{
				Name:        c.Name,
				Aliases:     c.Aliases,
				Description: c.Description,
				IsPublic:    c.IsPublic,
			})
		}
		return meta.BuildRunCommandInstructionWithName(metaCmds, botName)
	})

	pushName := ""
	msgID := ""
	if ctx.Evt != nil {
		if ctx.Evt.Info.PushName != "" {
			pushName = ctx.Evt.Info.PushName
		}
		msgID = ctx.Evt.Info.ID
	}

	data := meta.Data{
		ChatID:    ctx.Chat.String(),
		Question:  ctx.RawArgs,
		MessageID: msgID,
		User:      ctx.Sender,
		PushName:  pushName,
		IsSudo:    ctx.IsSudo(),
	}

	isGroup := ctx.Chat.Server == "g.us"

	if isGroup {
		data.ChatType = "group"
		groupInfo, err := meta.GetOrFetchGroupMeta(ctx.Chat.String(), func() (types.GroupInfo, error) {
			info, err := ctx.Client.GetGroupInfo(ctx.Ctx, ctx.Chat)
			if err != nil || info == nil {
				return types.GroupInfo{}, err
			}
			return *info, nil
		})
		if err == nil {
			data.GroupMetaData = groupInfo
		}
	} else {
		data.ChatType = "direct"
	}

	// Populate quoted-message context (extract context), if this message is a reply.
	extractContextFromQuotedMessage(ctx, &data)

	// Assemble the full query sent to Meta AI.
	query := instruction
	if s, okStore := ctx.Client.Store.Identities.(*sqlstore.SQLStore); okStore {
		if customPrompt, _ := s.GetSetting(ctx.Ctx, "csai_prompt"); customPrompt != "" {
			query += fmt.Sprintf("\n\n[GLOBAL BOT PERSONALITY & RELATIONSHIP BEHAVIOR INSTRUCTION]\n%s\n\n", customPrompt)
		}
	}
	if isGroup {
		query += meta.RenderGroupContext(data.GroupMetaData)
	}
	query += meta.RenderUserContext(data)
	query += meta.RenderQuotedContext(data)
	query += data.Question

	slog.Debug("handleAI: sending request to Meta AI", "chat", ctx.Chat.String())

	loader := ctx.StartLoader("Thinking")
	defer loader.Stop()
	placeholderMsgID := loader.MessageID()

	onUpdate := func(text string) error {
		trimmed := strings.TrimSpace(text)
		if trimmed == "" {
			return nil
		}
		if _, _, ok := meta.ParseRunCommand(trimmed); ok {
			return nil
		}
		loader.Stop()
		_, err := ctx.Edit(placeholderMsgID, text)
		if err != nil {
			slog.Error("handleAI: failed to send edit", "chat", ctx.Chat.String(), "err", err)
		}
		return err
	}

	res, err := queryMetaAi(ctx.Ctx, ctx.Client, ctx.Chat, query, onUpdate)
	if err != nil {
		loader.Stop()
		slog.Error("handleAI: queryMetaAi failed", "chat", ctx.Chat.String(), "err", err)
		if strings.Contains(err.Error(), "488") {
			errMsg := "Meta AI session initialization required.\n\nPlease make sure you have manually started a direct 1-on-1 chat/conversation with Meta AI on WhatsApp first before WhatsRook can interact with it."
			_, _ = ctx.Edit(placeholderMsgID, errMsg)

			// Send Meta AI contact card
			metaName := "Meta AI"
			metaVcard := fmt.Sprintf("BEGIN:VCARD\nVERSION:3.0\nN:AI;Meta;;;\nFN:%s\nTEL;type=CELL;waid=%s:+%s\nEND:VCARD", metaName, metaAiBotJID.User, metaAiBotJID.User)
			contactMsg := &waE2E.Message{
				ContactMessage: &waE2E.ContactMessage{
					DisplayName: &metaName,
					Vcard:       &metaVcard,
				},
			}
			_, _ = ctx.Client.SendMessage(ctx.Ctx, ctx.Chat, contactMsg)
			return err
		}

		_, _ = ctx.Edit(placeholderMsgID, "Failed to get a response: "+err.Error())
		return err
	}

	reply := res.Text

	if len(res.GeneratedImg) > 0 {
		mType := res.ImgMimeType
		if mType == "" {
			mType = "image/jpeg"
		}
		slog.Debug("handleAI: forwarding generated image to chat", "chat", ctx.Chat.String(), "img_len", len(res.GeneratedImg))
		_ = ctx.SendImage(res.GeneratedImg, mType, res.ImgCaption)
	}

	// Check whether the final reply is a RUN_COMMAND request.
	if cmdName, rawArgs, ok := meta.ParseRunCommand(reply); ok {
		if cmdName == "sh" || cmdName == "exec" || cmdName == "run" || cmdName == "shell" {
			if !ctx.IsSudo() {
				slog.Warn("handleAI: blocked unauthorized shell execution request", "sender", ctx.Sender.String())
				_, _ = ctx.Edit(placeholderMsgID, "You are not authorized to run shell commands.")
				return nil
			}

			output, err := meta.RunCmd(rawArgs)
			if err != nil && output == "" {
				output = err.Error()
			}
			if output == "" {
				output = "(no output)"
			}

			resText := fmt.Sprintf("Output:\n```\n%s\n```", output)
			_, err = ctx.Edit(placeholderMsgID, resText)
			return err
		}

		if cmdName == "ai" || cmdName == "autoai" || cmdName == "gpt" || cmdName == "ask" {
			slog.Warn("handleAI: blocked recursive AI command execution", "command", cmdName)
			_, err := ctx.Edit(placeholderMsgID, "Recursive AI command execution is not allowed.")
			return err
		}

		targetCmd, exists := Get(cmdName)
		if !exists {
			slog.Warn("handleAI: RUN_COMMAND referenced unknown command", "command", cmdName)
			_, _ = ctx.Edit(placeholderMsgID, "Sorry, I don't have a command called \""+cmdName+"\".")
			return nil
		}

		if !targetCmd.IsPublic && !ctx.IsSudo() {
			slog.Warn("handleAI: blocked unauthorized RUN_COMMAND", "sender", ctx.Sender.String(), "command", cmdName)
			_, _ = ctx.Edit(placeholderMsgID, "You are not authorized to run this command.")
			return nil
		}

		cctx := &Context{
			Ctx:     ctx.Ctx,
			Client:  ctx.Client,
			Evt:     ctx.Evt,
			Command: cmdName,
			Args:    strings.Fields(rawArgs),
			RawArgs: rawArgs,
			Chat:    ctx.Chat,
			Sender:  ctx.Sender,
		}
		slog.Debug("handleAI: executing command on behalf of AI", "command", cmdName, "args", ctx.Args)
		return targetCmd.Handler(cctx)
	}

	slog.Debug("handleAI: completed successfully", "chat", ctx.Chat.String())
	return nil
}

func extractContextFromQuotedMessage(ctx *Context, data *meta.Data) {
	if ctx.Evt == nil {
		return
	}
	quotedMsg := getQuotedMessageFromEvent(ctx.Evt)
	if quotedMsg == nil {
		return
	}

	// Extract Quoted Sender / Participant
	var quotedParticipant string
	msg := ctx.Evt.Message
	var ci *waE2E.ContextInfo
	switch {
	case msg.GetExtendedTextMessage() != nil:
		ci = msg.GetExtendedTextMessage().GetContextInfo()
	case msg.GetImageMessage() != nil:
		ci = msg.GetImageMessage().GetContextInfo()
	case msg.GetVideoMessage() != nil:
		ci = msg.GetVideoMessage().GetContextInfo()
	case msg.GetAudioMessage() != nil:
		ci = msg.GetAudioMessage().GetContextInfo()
	case msg.GetDocumentMessage() != nil:
		ci = msg.GetDocumentMessage().GetContextInfo()
	case msg.GetStickerMessage() != nil:
		ci = msg.GetStickerMessage().GetContextInfo()
	}
	if ci != nil {
		quotedParticipant = ci.GetParticipant()
		if ci.StanzaID != nil {
			data.QuotedMessageID = *ci.StanzaID
		}
	}

	if quotedParticipant != "" {
		if quotedJID, err := types.ParseJID(quotedParticipant); err == nil {
			data.UserOfQuotedMessage = quotedJID.User
			if data.ChatType == "group" {
				for _, p := range data.GroupMetaData.Participants {
					if p.JID.User == quotedJID.User {
						switch {
						case p.IsSuperAdmin:
							data.QuotedMessageParticipantRole = "Super Admin"
						case p.IsAdmin:
							data.QuotedMessageParticipantRole = "Admin"
						default:
							data.QuotedMessageParticipantRole = "Member"
						}
						break
					}
				}
			}
		}
	}

	switch {
	case quotedMsg.GetConversation() != "":
		data.QuotedMessageType = "Text"
		data.QuotedMessageOfQuestion = quotedMsg.GetConversation()

	case quotedMsg.GetExtendedTextMessage() != nil:
		data.QuotedMessageType = "Text"
		data.QuotedMessageOfQuestion = quotedMsg.GetExtendedTextMessage().GetText()

	case quotedMsg.GetImageMessage() != nil:
		imgMsg := quotedMsg.GetImageMessage()
		data.QuotedMessageType = "Image"
		data.QuotedMessageOfQuestion = imgMsg.GetCaption()
		mimetype := imgMsg.GetMimetype()
		if mimetype == "" {
			mimetype = "image/jpeg"
		}
		data.QuotedImageMimeType = mimetype

		imgData, err := ctx.Client.Download(ctx.Ctx, imgMsg)
		if err == nil && len(imgData) > 0 {
			data.QuotedImageBase64 = base64.StdEncoding.EncodeToString(imgData)
			slog.Debug("extractContextFromQuotedMessage: extracted image base64", "len", len(data.QuotedImageBase64))
		} else {
			slog.Warn("extractContextFromQuotedMessage: failed to download quoted image", "err", err)
		}

	case quotedMsg.GetVideoMessage() != nil:
		vidMsg := quotedMsg.GetVideoMessage()
		data.QuotedMessageType = "Video"
		caption := vidMsg.GetCaption()
		if caption != "" {
			data.QuotedMessageOfQuestion = fmt.Sprintf("[Video message. Note: Video file reading is not supported yet. Caption: %s]", caption)
		} else {
			data.QuotedMessageOfQuestion = "[Video message. Note: Video file reading is not supported yet.]"
		}

	case quotedMsg.GetAudioMessage() != nil:
		data.QuotedMessageType = "Audio"
		data.QuotedMessageOfQuestion = "[Voice/Audio message]"

	case quotedMsg.GetDocumentMessage() != nil:
		docMsg := quotedMsg.GetDocumentMessage()
		data.QuotedMessageType = "Document"
		caption := docMsg.GetCaption()
		filename := docMsg.GetFileName()
		if filename != "" {
			data.QuotedMessageOfQuestion = fmt.Sprintf("File: %s. Caption: %s", filename, caption)
		} else {
			data.QuotedMessageOfQuestion = caption
		}

	case quotedMsg.GetStickerMessage() != nil:
		stkMsg := quotedMsg.GetStickerMessage()
		if stkMsg.GetIsAnimated() {
			data.QuotedMessageType = "Animated Sticker"
			data.QuotedMessageOfQuestion = "[Animated/Video sticker message. Note: Animated or video stickers are not supported yet.]"
		} else {
			data.QuotedMessageType = "Sticker"
			data.QuotedMessageOfQuestion = "[Sticker image]"
			mimetype := stkMsg.GetMimetype()
			if mimetype == "" {
				mimetype = "image/webp"
			}
			data.QuotedImageMimeType = mimetype

			stkData, err := ctx.Client.Download(ctx.Ctx, stkMsg)
			if err == nil && len(stkData) > 0 {
				data.QuotedImageBase64 = base64.StdEncoding.EncodeToString(stkData)
				slog.Debug("extractContextFromQuotedMessage: extracted sticker image base64", "len", len(data.QuotedImageBase64))
			} else {
				slog.Warn("extractContextFromQuotedMessage: failed to download quoted sticker image", "err", err)
			}
		}

	case quotedMsg.GetPollCreationMessage() != nil || quotedMsg.GetPollCreationMessageV2() != nil || quotedMsg.GetPollCreationMessageV3() != nil:
		data.QuotedMessageType = "Poll"
		var pollName string
		var options []string
		if p := quotedMsg.GetPollCreationMessage(); p != nil {
			pollName = p.GetName()
			for _, opt := range p.GetOptions() {
				options = append(options, opt.GetOptionName())
			}
		} else if p := quotedMsg.GetPollCreationMessageV2(); p != nil {
			pollName = p.GetName()
			for _, opt := range p.GetOptions() {
				options = append(options, opt.GetOptionName())
			}
		} else if p := quotedMsg.GetPollCreationMessageV3(); p != nil {
			pollName = p.GetName()
			for _, opt := range p.GetOptions() {
				options = append(options, opt.GetOptionName())
			}
		}
		data.QuotedMessageOfQuestion = fmt.Sprintf("Poll Question: %s. Options: %s", pollName, strings.Join(options, ", "))

	case quotedMsg.GetLocationMessage() != nil:
		locMsg := quotedMsg.GetLocationMessage()
		data.QuotedMessageType = "Location"
		data.QuotedMessageOfQuestion = fmt.Sprintf("Location: %f, %f (%s)", locMsg.GetDegreesLatitude(), locMsg.GetDegreesLongitude(), locMsg.GetName())

	case quotedMsg.GetContactMessage() != nil:
		contMsg := quotedMsg.GetContactMessage()
		data.QuotedMessageType = "Contact"
		data.QuotedMessageOfQuestion = fmt.Sprintf("Contact: %s", contMsg.GetDisplayName())

	default:
		if txt := extractTextFromProto(quotedMsg); txt != "" {
			data.QuotedMessageType = "Other"
			data.QuotedMessageOfQuestion = txt
		}
	}
}
