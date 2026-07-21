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

// queryMetaAi sends request to Meta AI's bot JID and streams Meta AI's
// response back to the caller as it arrives.
//
// Meta AI streams its answer by sending an initial placeholder message and
// then repeatedly editing that same message (protocolMessage, type=14,
// key.ID pointing back to its own first message) until a final edit whose
// MsgBotInfo.EditType == "last" arrives. queryMetaAi:
//  1. Sends request as a plain text message to metaAiBotJID.
//  2. Waits for an incoming message from metaAiBotJID whose
//     MsgMetaInfo.TargetID matches the ID of the message just sent — this
//     is how Meta AI correlates its reply to our outgoing message.
//  3. Tracks further edits to that reply message (matched by the edit's
//     protocolMessage.Key.ID) and calls onUpdate with the latest text on
//     every edit.
//  4. Returns the text from the edit whose EditType == "last".
//
// Only one in-flight request per chat is allowed at a time; if a request
// for chat is already running, queryMetaAi returns ErrMetaAiBusy
// immediately without sending anything. If ctx is done before a final
// response arrives, queryMetaAi returns ctx.Err().
//
// onUpdate is called synchronously for every partial and the final
// update; pass nil to skip streaming and just get the final text back.
// If onUpdate returns an error, queryMetaAi stops and returns that error.
func queryMetaAi(ctx context.Context, client *whatsmeow.Client, chat types.JID, request string, onUpdate func(text string) error) (string, error) {
	chatKey := chat.String()

	if !tryLockMetaAi(chatKey) {
		return "", ErrMetaAiBusy
	}
	defer unlockMetaAi(chatKey)

	sentResp, err := client.SendMessage(ctx, metaAiBotJID, &waE2E.Message{
		Conversation: proto.String(request),
	})
	if err != nil {
		return "", fmt.Errorf("failed to send request to meta ai: %w", err)
	}
	sentID := sentResp.ID

	type update struct {
		text     string
		editType string
	}
	updates := make(chan update, 16)

	var (
		mu            sync.Mutex
		metaMsgID     string
		metaMsgIDSeen bool
	)

	handlerID := client.AddEventHandler(func(evt any) {
		msgEvt, ok := evt.(*events.Message)
		if !ok {
			return
		}
		if msgEvt.Info.Sender.String() != metaAiBotJID.String() {
			return
		}

		mu.Lock()
		if !metaMsgIDSeen {
			if msgEvt.Info.MsgMetaInfo.TargetID == sentID {
				metaMsgID = msgEvt.Info.ID
				metaMsgIDSeen = true
			} else {
				mu.Unlock()
				return
			}
		}
		expectedID := metaMsgID
		mu.Unlock()

		pm := msgEvt.Message.GetProtocolMessage()

		var text string
		var editType string

		if msgEvt.Info.ID == expectedID && pm == nil {
			// The very first message (not yet an edit) — usually the
			// "Thinking" placeholder. Surface it as an initial update.
			rich := msgEvt.Message.GetRichResponseMessage()
			if rich == nil {
				return
			}
			for _, sub := range rich.GetSubmessages() {
				text += sub.GetMessageText()
			}
			editType = string(msgEvt.Info.MsgBotInfo.EditType)
		} else {
			if pm == nil || pm.GetKey().GetID() != expectedID {
				return
			}
			edited := pm.GetEditedMessage()
			if edited == nil {
				return
			}
			rich := edited.GetRichResponseMessage()
			if rich == nil {
				return
			}
			for _, sub := range rich.GetSubmessages() {
				text += sub.GetMessageText()
			}
			editType = string(msgEvt.Info.MsgBotInfo.EditType)
		}

		select {
		case updates <- update{text: text, editType: editType}:
		default:
		}
	})
	defer client.RemoveEventHandler(handlerID)

	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case u := <-updates:
			if onUpdate != nil {
				if err := onUpdate(u.text); err != nil {
					return "", err
				}
			}
			if u.editType == "last" {
				return u.text, nil
			}
		}
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
		return err
	}

	_, err = queryMetaAi(ctx.Ctx, ctx.Client, ctx.Chat, query, onUpdate)
	if err != nil {
		if err == ErrMetaAiBusy {
			return sendText(ctx, "Please wait while I process another request.")
		}
		slog.Error("handleAI: queryMetaAi failed", "err", err)
		editMsg := ctx.Client.BuildEdit(ctx.Chat, placeholderResp.ID, &waE2E.Message{
			Conversation: proto.String("Failed to get a response: " + err.Error()),
		})
		_, _ = ctx.Client.SendMessage(ctx.Ctx, ctx.Chat, editMsg)
		return err
	}

	return nil
}