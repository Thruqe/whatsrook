package commands

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/Thruqe/whatsrook/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
)

type AIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type AIChatRequest struct {
	Messages  []AIMessage `json:"messages"`
	Model     string      `json:"model"`
	Cost      int         `json:"cost"`
	Stream    bool        `json:"stream"`
	WebSearch bool        `json:"web_search"`
}

var (
	aiHistory   = make(map[string][]AIMessage)
	aiHistoryMu sync.Mutex
	dbInitOnce  sync.Once
)

func init() {
	Register(&Command{
		Name:        "ai",
		Aliases:     []string{"gpt", "ask"},
		Description: "Ask the AI assistant a question. Use '!ai clear' to reset conversation history.",
		Category:    "AI",
		IsPublic:    true,
		Handler:     handleAI,
	})
}

func resolveContactName(ctx *Context, jid types.JID, pushName string) string {
	if contact, err := ctx.Client.Store.Contacts.GetContact(ctx.Ctx, jid); err == nil && contact.Found {
		if contact.FullName != "" {
			return contact.FullName
		}
		if contact.FirstName != "" {
			return contact.FirstName
		}
		if contact.PushName != "" {
			return contact.PushName
		}
	}
	if pushName != "" {
		return pushName
	}
	return jid.User
}

func handleAI(ctx *Context) error {
	slog.Info("handleAI started", "args", ctx.Args)

	if len(ctx.Args) == 0 {
		slog.Warn("handleAI: no query provided")
		return sendText(ctx, "Usage: !ai <question> (or reply to a message with !ai)")
	}

	query := ctx.RawArgs
	chatKey := ctx.Chat.String()

	// Get database connection if available
	var db *sql.DB
	if sqs, ok := ctx.Client.Store.Contacts.(*sqlstore.SQLStore); ok {
		db = sqs.GetDB().RawDB
	}

	if db != nil {
		dbInitOnce.Do(func() {
			_, err := db.Exec(`CREATE TABLE IF NOT EXISTS ai_history (
				chat_jid TEXT,
				role TEXT,
				content TEXT,
				timestamp DATETIME DEFAULT CURRENT_TIMESTAMP
			)`)
			if err != nil {
				slog.Error("failed to create ai_history table", "err", err)
			}
		})
	}

	// Check if they want to clear history
	if strings.EqualFold(query, "clear") {
		if db != nil {
			_, err := db.ExecContext(ctx.Ctx, "DELETE FROM ai_history WHERE chat_jid = ?", chatKey)
			if err != nil {
				slog.Error("failed to clear history from db", "err", err)
			}
		} else {
			aiHistoryMu.Lock()
			delete(aiHistory, chatKey)
			aiHistoryMu.Unlock()
		}
		slog.Info("handleAI: cleared conversation history", "chat", chatKey)
		return sendText(ctx, "AI conversation history cleared.")
	}

	// Retrieve existing history
	var history []AIMessage
	if db != nil {
		// Clean up records older than 48 hours
		_, _ = db.ExecContext(ctx.Ctx, "DELETE FROM ai_history WHERE timestamp < datetime('now', '-48 hours')")

		rows, err := db.QueryContext(ctx.Ctx, "SELECT role, content FROM ai_history WHERE chat_jid = ? ORDER BY timestamp DESC LIMIT 10", chatKey)
		if err == nil {
			defer rows.Close()
			var temp []AIMessage
			for rows.Next() {
				var m AIMessage
				if err := rows.Scan(&m.Role, &m.Content); err == nil {
					temp = append(temp, m)
				}
			}
			if err := rows.Err(); err != nil {
				slog.Error("error iterating ai_history rows", "err", err)
			}
			// Reverse temp to restore chronological order
			for _, m := range slices.Backward(temp) {
				history = append(history, m)
			}
		} else {
			slog.Error("failed to query ai_history from db", "err", err)
		}
	} else {
		aiHistoryMu.Lock()
		history = aiHistory[chatKey]
		aiHistoryMu.Unlock()
	}

	// Format user query with sender metadata so AI can distinguish participants
	senderName := resolveContactName(ctx, ctx.Sender, ctx.Evt.Info.PushName)
	formattedQuery := fmt.Sprintf("[%s (%s)]: %s", senderName, ctx.Sender.User, query)

	// Append user question
	history = append(history, AIMessage{Role: "user", Content: formattedQuery})

	// Limit history size to last 10 messages (only needed for fallback memory)
	if db == nil && len(history) > 10 {
		history = history[len(history)-10:]
	}

	// Construct a rich system prompt with sender and group metadata
	currentTime := time.Now().Format("2006-01-02 15:04:05 MST")

	privilege := "Regular User"
	if ctx.IsSudo() {
		privilege = "Owner/Sudoer"
	}

	var groupName, groupTopic, groupJID string
	var participantCount int
	var groupAdminsList string
	var groupCreatedStr string
	var groupOwnerStr string
	var groupParticipantsStr string
	isGroup := ctx.Chat.Server == "g.us"

	if isGroup {
		groupInfo, err := ctx.Client.GetGroupInfo(ctx.Ctx, ctx.Chat)
		if err == nil && groupInfo != nil {
			groupName = groupInfo.GroupName.Name
			groupTopic = groupInfo.GroupTopic.Topic
			participantCount = groupInfo.ParticipantCount
			groupJID = groupInfo.JID.String()

			if !groupInfo.GroupCreated.IsZero() {
				groupCreatedStr = groupInfo.GroupCreated.Format("2006-01-02 15:04:05 MST")
			}

			if !groupInfo.OwnerJID.IsEmpty() {
				resolvedOwner, _ := ctx.ResolveMention(groupInfo.OwnerJID)
				ownerName := resolveContactName(ctx, groupInfo.OwnerJID, "")
				groupOwnerStr = fmt.Sprintf("%s (%s)", ownerName, resolvedOwner.User)
			} else if !groupInfo.OwnerPN.IsEmpty() {
				ownerName := resolveContactName(ctx, groupInfo.OwnerPN, "")
				groupOwnerStr = fmt.Sprintf("%s (%s)", ownerName, groupInfo.OwnerPN.User)
			}

			adminNames := make([]string, 0)
			participantsList := make([]string, 0, len(groupInfo.Participants))
			for _, p := range groupInfo.Participants {
				var resolvedJID types.JID
				if !p.PhoneNumber.IsEmpty() {
					resolvedJID = p.PhoneNumber.ToNonAD()
				} else {
					resolvedJID, _ = ctx.ResolveMention(p.JID)
				}
				name := resolveContactName(ctx, p.JID, "")

				role := "Member"
				if p.IsSuperAdmin {
					role = "Super Admin"
				} else if p.IsAdmin {
					role = "Admin"
				}

				participantsList = append(participantsList, fmt.Sprintf("- %s (Phone/Number: %s, Role: %s)", name, resolvedJID.User, role))

				if p.IsAdmin || p.IsSuperAdmin {
					adminNames = append(adminNames, fmt.Sprintf("%s (%s)", name, resolvedJID.User))
					if p.JID.ToNonAD() == ctx.Sender.ToNonAD() {
						if privilege != "Owner/Sudoer" {
							privilege = "Group Admin"
						}
					}
				}
			}
			groupAdminsList = strings.Join(adminNames, ", ")
			groupParticipantsStr = strings.Join(participantsList, "\n")
		}
	}

	var botCommands []string
	for _, c := range Visible() {
		botCommands = append(botCommands, fmt.Sprintf("- !%s: %s (Aliases: %s)", c.Name, c.Description, strings.Join(c.Aliases, ", ")))
	}
	botCommandsList := strings.Join(botCommands, "\n")

	systemPrompt := fmt.Sprintf(
		"You are a smart, helpful WhatsApp bot assistant. Here is the metadata context of the user sending the message and the chat room:\n"+
			"- Sender Name: %s\n"+
			"- Sender Phone/ID: %s\n"+
			"- Sender JID: %s\n"+
			"- Sender Privilege/Role: %s\n"+
			"- Current Local Time: %s\n",
		senderName, ctx.Sender.User, ctx.Sender.String(), privilege, currentTime,
	)

	if isGroup {
		systemPrompt += fmt.Sprintf(
			"- Chat Type: Group Chat\n"+
				"- Group Name: %s\n"+
				"- Group Description: %s\n"+
				"- Group Participant Count: %d\n"+
				"- Group Admins: %s\n"+
				"- Group Owner/Founder: %s\n"+
				"- Group Created At: %s\n"+
				"- Group JID: %s\n"+
				"- Group Participants:\n%s\n",
			groupName, groupTopic, participantCount, groupAdminsList, groupOwnerStr, groupCreatedStr, groupJID, groupParticipantsStr,
		)
	} else {
		systemPrompt += "- Chat Type: Direct Message (Private Chat)\n"
	}

	systemPrompt += fmt.Sprintf(
		"\nAvailable Bot Commands users can run directly:\n%s\n"+
			"\nCRITICAL RULES FOR RESPONDING:\n"+
			"1. Keep responses extremely direct, short, and to the point. Do NOT start your response with greeting introductions or list what you can do (e.g. \"I can help you with...\") unless the user explicitly asks for help or asks what you can do. Do NOT add friendly follow-ups (e.g. \"Is there anything else I can help you with?\") at the end of responses. Avoid conversational filler entirely.\n"+
			"2. You are talking to ordinary WhatsApp users. Do NOT mention internal code concepts, Go functions, variables, structures, database tables, or developers' terms (e.g., do NOT mention 'client.groupMetadata', 'sqlstore', 'creation timestamp field', etc.).\n"+
			"3. If a user asks a question about the group (like participant lists, creation date, admin lists, etc.), answer them directly using the metadata provided above. For example, if they ask when the group was created, look at 'Group Created At' above. If they ask about participants, check the list. Do not tell them to use code or APIs.\n"+
			"4. If they need to perform an action (e.g., mute, kick, get invite link) or if they want to run a specific command, point them to the user-facing bot commands listed above.\n"+
			"5. If a piece of information is not available in the metadata, suggest they check standard WhatsApp group info or options in their WhatsApp application, or use the appropriate bot commands.\n"+
			"6. Write in a completely natural, human-like, conversational tone. Do NOT output raw metadata lists, and do NOT copy the labels/keys from the system prompt context (e.g., do NOT format responses as: 'Sender Name: ... — Phone/ID: ... — Role: ...'). Translate them into a natural sentence (e.g. 'You are Romania Dude (28024745529539), and you are a regular member in the WASocket Support group.').\n"+
			"7. Do NOT output unrelated robotic placeholders or headers (e.g., do NOT output 'Model name: Not disclosed' or similar text) unless the user explicitly asked about the AI model name.\n"+
			"8. If the user asks you to perform an action supported by an available bot command (such as tagging everyone, kicking a user, promoting/demoting, adding a user, checking CPU/memory, etc.), you can execute that command by returning exactly: 'RUN_COMMAND: !<command_name> [args]'. For example, if they say 'Tag every user here' or 'tag everyone', output exactly 'RUN_COMMAND: !tagall'. Do not include any other conversational text in your reply when you output RUN_COMMAND.\n"+
			"9. If you want to tag a specific user in your conversational text response, use '@' followed by their Phone/Number or user ID (e.g. '@28024745529539'). The system will automatically convert this to a real WhatsApp tag/mention. Do NOT use their names for the mention, use their Phone/Number JID.\n"+
			"10. You can run system shell commands when requested by the user by returning exactly: 'RUN_COMMAND: !sh <exact_command_requested>'. You MUST run the exact shell command that the user asked you to run. Do NOT copy examples from this rule list. Only execute shell commands when specifically requested or necessary to answer the user's question, and do not include any other conversational text when outputting RUN_COMMAND.\n"+
			"11. SENSITIVE DATA SECURITY: You MUST check the user's privilege/role before executing shell commands or disclosing host system/environment details. If 'Sender Privilege/Role' is NOT 'Owner/Sudoer', you MUST refuse to execute shell commands, retrieve system information, or disclose any host/server details. Respond that you cannot perform this action because it is restricted to sudoers.\n"+
			"12. PLAIN TEXT FORMATTING & NO EMOJIS: Always format your responses as plain text. Do NOT use emojis, markdown formatting, bolding, italicizing, or code block formatting unless the user explicitly requested rich formatting, markdown, or emojis in their query. Avoid long blocks of text.\n",
		botCommandsList,
	)

	// Prepare actual request messages prepending the dynamic system prompt
	reqMessages := make([]AIMessage, 0, len(history)+1)
	reqMessages = append(reqMessages, AIMessage{Role: "system", Content: systemPrompt})
	reqMessages = append(reqMessages, history...)

	slog.Info("handleAI: calling AI API", "chat", chatKey, "history_len", len(history))

	// Send typing presence indicator
	_ = ctx.Client.SendChatPresence(ctx.Ctx, ctx.Chat, types.ChatPresenceComposing, types.ChatPresenceMediaText)

	reply, err := queryAI(ctx.Ctx, reqMessages)
	if err != nil {
		slog.Error("handleAI: AI API query failed", "err", err)
		errMsg := err.Error()
		if strings.Contains(errMsg, "deadline exceeded") || strings.Contains(errMsg, "timeout") || strings.Contains(errMsg, "Timeout") {
			slog.Warn("handleAI: timeout/deadline exceeded, clearing history and retrying without history", "chat", chatKey)
			if db != nil {
				_, _ = db.ExecContext(ctx.Ctx, "DELETE FROM ai_history WHERE chat_jid = ?", chatKey)
			} else {
				aiHistoryMu.Lock()
				delete(aiHistory, chatKey)
				aiHistoryMu.Unlock()
			}

			// Retry with just the system prompt and the user's formatted query
			retryMessages := []AIMessage{
				{Role: "system", Content: systemPrompt},
				{Role: "user", Content: formattedQuery},
			}

			_ = ctx.Client.SendChatPresence(ctx.Ctx, ctx.Chat, types.ChatPresenceComposing, types.ChatPresenceMediaText)
			reply, err = queryAI(ctx.Ctx, retryMessages)
			if err != nil {
				slog.Error("handleAI: AI API retry query failed", "err", err)
				return sendText(ctx, "Failed to get response from AI even after clearing history: "+err.Error())
			}

			reply = " Notice: Chat history was cleared to resolve a timeout limit.\n\n" + reply
		} else {
			return sendText(ctx, "Failed to get response from AI: "+errMsg)
		}
	}

	slog.Info("handleAI: AI response received", "reply_len", len(reply))

	// Check if AI response has a request to run a command
	cleanReply := strings.TrimSpace(reply)
	if cmdContent, ok := strings.CutPrefix(cleanReply, "RUN_COMMAND:"); ok {
		cmdLine := strings.TrimSpace(cmdContent)
		cmdLineClean := strings.TrimLeft(cmdLine, ".!/ ")
		fields := strings.Fields(cmdLineClean)
		if len(fields) > 0 {
			cmdName := strings.ToLower(fields[0])
			cmdArgs := fields[1:]
			cmdRawArgs := strings.TrimSpace(cmdLineClean[len(fields[0]):])

			if targetCmd, ok := Get(cmdName); ok {
				if !targetCmd.IsPublic && !ctx.IsSudo() {
					slog.Warn("handleAI: blocked unauthorized command from AI response", "sender", ctx.Sender.String(), "command", cmdName)
					return sendText(ctx, " You are not authorized to run system commands.")
				}

				cctx := &Context{
					Ctx:     ctx.Ctx,
					Client:  ctx.Client,
					Evt:     ctx.Evt,
					Command: cmdName,
					Args:    cmdArgs,
					RawArgs: cmdRawArgs,
					Chat:    ctx.Chat,
					Sender:  ctx.Sender,
				}
				slog.Info("handleAI: executing command on behalf of AI", "command", cmdName, "args", cmdArgs)

				// Save history first
				if db != nil {
					_, _ = db.ExecContext(ctx.Ctx, "INSERT INTO ai_history (chat_jid, role, content) VALUES (?, 'user', ?)", chatKey, formattedQuery)
					_, _ = db.ExecContext(ctx.Ctx, "INSERT INTO ai_history (chat_jid, role, content) VALUES (?, 'assistant', ?)", chatKey, reply)
				} else {
					history = append(history, AIMessage{Role: "assistant", Content: reply})
					aiHistoryMu.Lock()
					aiHistory[chatKey] = history
					aiHistoryMu.Unlock()
				}

				return targetCmd.Handler(cctx)
			}
		}
	}

	// Detect mentions in the reply to tag users properly
	var mentionedJIDs []types.JID
	if isGroup {
		groupInfo, err := ctx.Client.GetGroupInfo(ctx.Ctx, ctx.Chat)
		if err == nil && groupInfo != nil {
			for _, p := range groupInfo.Participants {
				var resolvedJID types.JID
				if !p.PhoneNumber.IsEmpty() {
					resolvedJID = p.PhoneNumber.ToNonAD()
				} else {
					resolvedJID, _ = ctx.ResolveMention(p.JID)
				}

				targetMention := "@" + resolvedJID.User
				if strings.Contains(reply, targetMention) {
					mentionedJIDs = append(mentionedJIDs, p.JID)
				}

				lidMention := "@" + p.JID.User
				if strings.Contains(reply, lidMention) && p.JID.User != resolvedJID.User {
					mentionedJIDs = append(mentionedJIDs, p.JID)
				}
			}
		}
	}

	// Save back history
	if db != nil {
		_, errUser := db.ExecContext(ctx.Ctx, "INSERT INTO ai_history (chat_jid, role, content) VALUES (?, 'user', ?)", chatKey, formattedQuery)
		_, errAssistant := db.ExecContext(ctx.Ctx, "INSERT INTO ai_history (chat_jid, role, content) VALUES (?, 'assistant', ?)", chatKey, reply)
		if errUser != nil || errAssistant != nil {
			slog.Error("failed to insert history to db", "errUser", errUser, "errAssistant", errAssistant)
		}
	} else {
		// Append assistant response to history
		history = append(history, AIMessage{Role: "assistant", Content: reply})
		aiHistoryMu.Lock()
		aiHistory[chatKey] = history
		aiHistoryMu.Unlock()
	}

	if len(mentionedJIDs) > 0 {
		return ctx.ReplyWithMentions(reply, mentionedJIDs)
	}
	return sendText(ctx, reply)
}

func generateUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40 // Version 4
	b[8] = (b[8] & 0x3f) | 0x80 // Variant RFC4122
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func saveChatSession(ctx context.Context, client *http.Client, sessionUUID string) error {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	_ = writer.WriteField("uuid", sessionUUID)
	_ = writer.WriteField("title", "")
	_ = writer.WriteField("chat_style", "gpt-chat")
	_ = writer.WriteField("messages", "[]")
	writer.Close()

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.deepai.org/save_chat_session", &body)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("origin", "https://deepai.org")
	req.Header.Set("referer", "https://deepai.org/chat/gpt-chat")
	req.Header.Set("user-agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/150.0.0.0 Safari/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to save chat session: status %d: %s", resp.StatusCode, string(b))
	}

	return nil
}

func queryAI(ctx context.Context, messages []AIMessage) (string, error) {
	type chatMessage struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}

	var systemContent string
	var chatHistory []chatMessage

	for _, msg := range messages {
		if msg.Role == "system" {
			systemContent = msg.Content
			continue
		}
		chatHistory = append(chatHistory, chatMessage{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	if systemContent != "" {
		if len(chatHistory) > 0 {
			chatHistory[0].Content = fmt.Sprintf("SYSTEM CONTEXT:\n%s\n\nUSER PROMPT:\n%s", systemContent, chatHistory[0].Content)
		} else {
			chatHistory = append(chatHistory, chatMessage{
				Role:    "user",
				Content: fmt.Sprintf("SYSTEM CONTEXT:\n%s", systemContent),
			})
		}
	}

	historyJSON, err := json.Marshal(chatHistory)
	if err != nil {
		return "", err
	}

	sessionUUID := generateUUID()
	reqID := generateUUID()

	client := &http.Client{Timeout: 60 * time.Second}

	_ = saveChatSession(ctx, client, sessionUUID)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	_ = writer.WriteField("chat_style", "gpt-chat")
	_ = writer.WriteField("chatHistory", string(historyJSON))
	_ = writer.WriteField("model", "standard")
	_ = writer.WriteField("session_uuid", sessionUUID)
	_ = writer.WriteField("sensitivity_request_id", reqID)
	_ = writer.WriteField("hacker_is_stinky", "very_stinky")
	_ = writer.WriteField("enabled_tools", `["image_generator","image_editor"]`)
	writer.Close()

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.deepai.org/hacking_is_a_serious_crime", &body)
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("api-key", "tryit-15577571150-bd0743084e2bc4d3ac4ef52f248f653b")
	req.Header.Set("origin", "https://deepai.org")
	req.Header.Set("referer", "https://deepai.org/chat/gpt-chat")
	req.Header.Set("user-agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/150.0.0.0 Safari/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	reply := strings.TrimSpace(string(respBytes))
	if reply == "" {
		return "", fmt.Errorf("empty response received from AI")
	}

	return reply, nil
}

func init() {
	Register(&Command{
		Name:        "autoai",
		Description: "Toggle automatic AI responses when the bot is tagged or replied to in this chat (on/off)",
		Category:    "AI",
		IsPublic:    true,
		Handler:     handleAutoAI,
	})
}

func handleAutoAI(ctx *Context) error {
	slog.Info("handleAutoAI started", "args", ctx.Args)

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
		return ctx.Reply(" Only sudoers or group admins can change the AutoAI setting.")
	}

	s, okStore := ctx.Client.Store.Identities.(*sqlstore.SQLStore)
	if !okStore {
		return ctx.Reply(" Database store is not available.")
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
		return ctx.Reply(" Usage: !autoai [on/off]")
	}

	if err := s.PutSetting(ctx.Ctx, settingKey, val); err != nil {
		slog.Error("failed to update autoai setting", "err", err)
		return ctx.Reply(" Failed to update setting: " + err.Error())
	}

	return ctx.Reply(fmt.Sprintf("AutoAI has been set to %s for this chat.", val))
}