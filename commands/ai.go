package commands

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

// metaAiBotJID is the fixed JID Meta AI's bot account is reached at.
var metaAiBotJID = types.NewJID("867051314767696", "bot")

// ErrMetaAiBusy is returned by queryMetaAi when a request for the same
// chat is already in progress.
var ErrMetaAiBusy = fmt.Errorf("a Meta AI request is already in progress for this chat; please wait")

var (
	metaAiInFlight   = make(map[string]bool)
	metaAiInFlightMu sync.Mutex
)

func tryLockMetaAi(chatKey string) bool {
	metaAiInFlightMu.Lock()
	defer metaAiInFlightMu.Unlock()
	if metaAiInFlight[chatKey] {
		return false
	}
	metaAiInFlight[chatKey] = true
	return true
}

func unlockMetaAi(chatKey string) {
	metaAiInFlightMu.Lock()
	defer metaAiInFlightMu.Unlock()
	delete(metaAiInFlight, chatKey)
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
		var text string
		for _, sub := range rich.GetSubmessages() {
			text += sub.GetMessageText()
		}
		return text
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
func queryMetaAi(ctx context.Context, client *whatsmeow.Client, chat types.JID, request string, onUpdate func(text string) error) (string, error) {
	chatKey := chat.String()

	if !tryLockMetaAi(chatKey) {
		slog.Warn("queryMetaAi: rejected, already in progress for chat", "chat", chatKey)
		return "", ErrMetaAiBusy
	}
	defer unlockMetaAi(chatKey)

	slog.Debug("queryMetaAi: sending request", "chat", chatKey, "request", request)

	if _, err := client.SendMessage(ctx, metaAiBotJID, &waE2E.Message{
		Conversation: proto.String(request),
	}); err != nil {
		slog.Error("queryMetaAi: failed to send request", "chat", chatKey, "err", err)
		return "", fmt.Errorf("failed to send request to meta ai: %w", err)
	}

	var (
		mu        sync.Mutex
		metaMsgID string
		seen      bool
		final     string
		done      = make(chan struct{})
		closeOnce sync.Once
	)

	handlerID := client.AddEventHandler(func(evt any) {
		msgEvt, ok := evt.(*events.Message)
		if !ok || msgEvt.Info.Sender.String() != metaAiBotJID.String() {
			return
		}

		pm := msgEvt.Message.GetProtocolMessage()

		mu.Lock()
		if !seen {
			if pm != nil {
				mu.Unlock()
				return
			}
			metaMsgID = msgEvt.Info.ID
			seen = true
			mu.Unlock()
			slog.Debug("queryMetaAi: captured meta ai reply message id", "chat", chatKey, "meta_msg_id", metaMsgID)
		} else if pm == nil || pm.GetKey().GetID() != metaMsgID {
			mu.Unlock()
			return
		} else {
			mu.Unlock()
		}

		var text string
		if pm == nil {
			text = extractMetaAiText(msgEvt.Message)
		} else {
			text = extractMetaAiText(pm.GetEditedMessage())
		}
		if text == "" {
			slog.Debug("queryMetaAi: empty text extracted, skipping update", "chat", chatKey, "info_id", msgEvt.Info.ID)
			return
		}

		editType := string(msgEvt.Info.MsgBotInfo.EditType)
		slog.Debug("queryMetaAi: update", "chat", chatKey, "edit_type", editType, "text", text)

		if onUpdate != nil {
			if err := onUpdate(text); err != nil {
				slog.Error("queryMetaAi: onUpdate callback failed", "chat", chatKey, "err", err)
			}
		}
		if editType == "last" {
			mu.Lock()
			final = text
			mu.Unlock()
			closeOnce.Do(func() { close(done) })
		}
	})
	defer client.RemoveEventHandler(handlerID)

	select {
	case <-ctx.Done():
		slog.Warn("queryMetaAi: context cancelled/timed out before completion", "chat", chatKey, "err", ctx.Err())
		return "", ctx.Err()
	case <-done:
		mu.Lock()
		defer mu.Unlock()
		slog.Info("queryMetaAi: completed", "chat", chatKey, "final_text_len", len(final))
		return final, nil
	}
}

func init() {
	Register(&Command{
		Name:        "ai",
		Aliases:     []string{"gpt", "ask"},
		Description: "Ask Meta AI a question.",
		Category:    "AI",
		IsPublic:    true,
		Handler:     handleAI,
	})
}

func handleAI(ctx *Context) error {
	if len(ctx.Args) == 0 {
		return sendText(ctx, "Usage: !ai <question>")
	}
	query := ctx.RawArgs

	slog.Info("handleAI: sending request to Meta AI", "chat", ctx.Chat.String())

	placeholderResp, err := ctx.Client.SendMessage(ctx.Ctx, ctx.Chat, &waE2E.Message{
		Conversation: proto.String("Thinking..."),
	})
	if err != nil {
		slog.Error("handleAI: failed to send placeholder message", "chat", ctx.Chat.String(), "err", err)
		return fmt.Errorf("failed to send placeholder message: %w", err)
	}

	onUpdate := func(text string) error {
		if strings.TrimSpace(text) == "" {
			return nil
		}
		editMsg := ctx.Client.BuildEdit(ctx.Chat, placeholderResp.ID, &waE2E.Message{
			Conversation: proto.String(text),
		})
		_, err := ctx.Client.SendMessage(ctx.Ctx, ctx.Chat, editMsg)
		if err != nil {
			slog.Error("handleAI: failed to send edit", "chat", ctx.Chat.String(), "err", err)
		}
		return err
	}

	_, err = queryMetaAi(ctx.Ctx, ctx.Client, ctx.Chat, query, onUpdate)
	if err != nil {
		if err == ErrMetaAiBusy {
			slog.Warn("handleAI: meta ai busy for chat", "chat", ctx.Chat.String())
			return sendText(ctx, "Please wait while I process another request.")
		}
		slog.Error("handleAI: queryMetaAi failed", "chat", ctx.Chat.String(), "err", err)
		editMsg := ctx.Client.BuildEdit(ctx.Chat, placeholderResp.ID, &waE2E.Message{
			Conversation: proto.String("Failed to get a response: " + err.Error()),
		})
		_, _ = ctx.Client.SendMessage(ctx.Ctx, ctx.Chat, editMsg)
		return err
	}

	slog.Info("handleAI: completed successfully", "chat", ctx.Chat.String())
	return nil
}
