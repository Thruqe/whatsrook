package commands

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

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

func handleAI(ctx *Context) error {
	slog.Info("handleAI started", "args", ctx.Args)

	if len(ctx.Args) == 0 {
		slog.Warn("handleAI: no query provided")
		return sendText(ctx, "Usage: !ai <question> (or reply to a message with !ai)")
	}

	query := ctx.RawArgs
	chatKey := ctx.Chat.String()

	// Check if they want to clear history
	if strings.EqualFold(query, "clear") {
		aiHistoryMu.Lock()
		delete(aiHistory, chatKey)
		aiHistoryMu.Unlock()
		slog.Info("handleAI: cleared conversation history", "chat", chatKey)
		return sendText(ctx, "AI conversation history cleared.")
	}

	// Retrieve existing history
	aiHistoryMu.Lock()
	history := aiHistory[chatKey]
	aiHistoryMu.Unlock()

	// Append user question
	history = append(history, AIMessage{Role: "user", Content: query})

	// Limit history size to last 10 messages to keep prompt size reasonable
	if len(history) > 10 {
		history = history[len(history)-10:]
	}

	// Construct a rich system prompt with sender and group metadata
	pushName := ctx.Evt.Info.PushName
	if pushName == "" {
		pushName = "Unknown"
	}
	senderPhone := ctx.Sender.User
	senderJID := ctx.Sender.String()
	currentTime := time.Now().Format("2006-01-02 15:04:05 MST")

	privilege := "Regular User"
	if ctx.IsSudo() {
		privilege = "Owner/Sudoer"
	}

	var groupName, groupTopic, groupJID string
	var participantCount int
	isGroup := ctx.Chat.Server == "g.us"

	if isGroup {
		groupInfo, err := ctx.Client.GetGroupInfo(ctx.Ctx, ctx.Chat)
		if err == nil && groupInfo != nil {
			groupName = groupInfo.GroupName.Name
			groupTopic = groupInfo.GroupTopic.Topic
			participantCount = groupInfo.ParticipantCount
			groupJID = groupInfo.JID.String()

			for _, p := range groupInfo.Participants {
				if p.JID.ToNonAD() == ctx.Sender.ToNonAD() {
					if p.IsAdmin || p.IsSuperAdmin {
						if privilege != "Owner/Sudoer" {
							privilege = "Group Admin"
						}
					}
					break
				}
			}
		}
	}

	systemPrompt := fmt.Sprintf(
		"You are a helpful WhatsApp AI assistant. Here is the metadata context of the user sending the message and the chat room:\n"+
			"- Sender Name: %s\n"+
			"- Sender Phone/ID: %s\n"+
			"- Sender JID: %s\n"+
			"- Sender Privilege/Role: %s\n"+
			"- Current Local Time: %s\n",
		pushName, senderPhone, senderJID, privilege, currentTime,
	)

	if isGroup {
		systemPrompt += fmt.Sprintf(
			"- Chat Type: Group Chat\n"+
				"- Group Name: %s\n"+
				"- Group Description: %s\n"+
				"- Group Participant Count: %d\n"+
				"- Group JID: %s\n",
			groupName, groupTopic, participantCount, groupJID,
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

	// Append assistant response to history
	history = append(history, AIMessage{Role: "assistant", Content: reply})

	// Save back history
	aiHistoryMu.Lock()
	aiHistory[chatKey] = history
	aiHistoryMu.Unlock()

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
