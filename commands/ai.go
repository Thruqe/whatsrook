package commands

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
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
			// Reverse temp to restore chronological order
			for i := len(temp) - 1; i >= 0; i-- {
				history = append(history, temp[i])
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
	isGroup := ctx.Chat.Server == "g.us"

	if isGroup {
		groupInfo, err := ctx.Client.GetGroupInfo(ctx.Ctx, ctx.Chat)
		if err == nil && groupInfo != nil {
			groupName = groupInfo.GroupName.Name
			groupTopic = groupInfo.GroupTopic.Topic
			participantCount = groupInfo.ParticipantCount
			groupJID = groupInfo.JID.String()

			adminNames := make([]string, 0)
			for _, p := range groupInfo.Participants {
				if p.IsAdmin || p.IsSuperAdmin {
					name := resolveContactName(ctx, p.JID, "")
					adminNames = append(adminNames, fmt.Sprintf("%s (%s)", name, p.JID.User))
					if p.JID.ToNonAD() == ctx.Sender.ToNonAD() {
						if privilege != "Owner/Sudoer" {
							privilege = "Group Admin"
						}
					}
				}
			}
			groupAdminsList = strings.Join(adminNames, ", ")
		}
	}

	systemPrompt := fmt.Sprintf(
		"You are a helpful WhatsApp AI assistant. Here is the metadata context of the user sending the message and the chat room:\n"+
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
				"- Group JID: %s\n",
			groupName, groupTopic, participantCount, groupAdminsList, groupJID,
		)
	} else {
		systemPrompt += "- Chat Type: Direct Message (Private Chat)\n"
	}

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
		return sendText(ctx, "Failed to get response from AI: "+err.Error())
	}

	slog.Info("handleAI: AI response received", "reply_len", len(reply))

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

	return sendText(ctx, reply)
}

func queryAI(ctx context.Context, messages []AIMessage) (string, error) {
	reqPayload := AIChatRequest{
		Messages:  messages,
		Model:     "v3",
		Cost:      1,
		Stream:    true,
		WebSearch: false,
	}

	jsonBytes, err := json.Marshal(reqPayload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://llmproxy.org/api/chat.php", bytes.NewBuffer(jsonBytes))
	if err != nil {
		return "", err
	}

	// Set headers exactly matching freegpt.ai requests
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://freegpt.ai")
	req.Header.Set("Referer", "https://freegpt.ai/")
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/150.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "*/*")

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	reader := bufio.NewReader(resp.Body)
	var assistantReply strings.Builder

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			return "", err
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "data: ") {
			dataContent := strings.TrimPrefix(line, "data: ")
			if dataContent == "[DONE]" {
				break
			}

			var chunk struct {
				Choices []struct {
					Delta struct {
						Content string `json:"content"`
					} `json:"delta"`
				} `json:"choices"`
			}
			if err := json.Unmarshal([]byte(dataContent), &chunk); err == nil {
				if len(chunk.Choices) > 0 {
					assistantReply.WriteString(chunk.Choices[0].Delta.Content)
				}
			}
		}
	}

	reply := assistantReply.String()
	if reply == "" {
		return "", fmt.Errorf("empty response received from AI")
	}

	return reply, nil
}
