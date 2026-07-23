// Abstract message builder for composing rich WhatsApp messages with
// formatting, quoting, mentions, and buttons.
package sender

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"whatsrook/font"
	"whatsrook/store/sqlstore"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

// FormatTextResponseRaw formats a text response with monospace format, removing asterisks and emojis unless it's already formatted.
func FormatTextResponseRaw(text string) string {
	text = strings.ReplaceAll(text, "*", "")
	text = removeEmojis(text)
	text = strings.ReplaceAll(text, "```", "")
	return font.Convert(text)
}

// formatTextResponse strips asterisks, emojis, and wraps response in 3 backticks for monospace formatting
func (ctx *Context) formatTextResponse(text string) string {
	return FormatTextResponseRaw(text)
}

// simulateTyping simulates text typing presence for 3 seconds
func (ctx *Context) simulateTyping() {
	_ = ctx.Client.SendChatPresence(ctx.Ctx, ctx.Chat, types.ChatPresenceComposing, types.ChatPresenceMediaText)
	time.Sleep(2 * time.Second)
}

// simulateRecording simulates audio recording presence for 3 seconds
func (ctx *Context) simulateRecording() {
	_ = ctx.Client.SendChatPresence(ctx.Ctx, ctx.Chat, types.ChatPresenceComposing, types.ChatPresenceMediaAudio)
	time.Sleep(3 * time.Second)
}

// SendText sends a simple text message to the current chat (with typing simulation and monospace format).
func (ctx *Context) SendText(text string) error {
	ctx.simulateTyping()
	formatted := ctx.formatTextResponse(text)
	slog.Debug("Building SendText", "text", text, "formatted", formatted)
	slog.Info("Sending SendText", "chat", ctx.Chat.String())
	_, err := ctx.Client.SendMessage(ctx.Ctx, ctx.Chat, &waE2E.Message{
		Conversation: &formatted,
	})
	if err != nil {
		slog.Error("SendText failed", "err", err)
	} else {
		slog.Info("SendText sent successfully")
	}
	return err
}

// Reply sends a text message replying to the current message (with typing simulation and monospace format).
func (ctx *Context) Reply(text string) error {
	ctx.simulateTyping()
	formatted := ctx.formatTextResponse(text)
	cinfo := ctx.replyContextInfo()
	slog.Debug("Building Reply", "text", text, "formatted", formatted, "context_info", cinfo)
	slog.Info("Sending Reply", "chat", ctx.Chat.String())
	_, err := ctx.Client.SendMessage(ctx.Ctx, ctx.Chat, &waE2E.Message{
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text:        &formatted,
			ContextInfo: cinfo,
		},
	})
	if err != nil {
		slog.Error("Reply failed", "err", err)
	} else {
		slog.Info("Reply sent successfully")
	}
	return err
}

func (ctx *Context) replyContextInfo() *waE2E.ContextInfo {
	if ctx.Evt == nil {
		return nil
	}
	stanzaID := ctx.Evt.Info.ID
	participant := ctx.Sender.ToNonAD().String()
	return &waE2E.ContextInfo{
		StanzaID:      &stanzaID,
		Participant:   &participant,
		QuotedMessage: ctx.Evt.Message,
	}
}

// SendImage uploads and sends an image to the current chat.
func (ctx *Context) SendImage(data []byte, mimetype, caption string) error {
	if mimetype == "" {
		slog.Warn("SendImage: mimetype is empty, defaulting to image/jpeg")
		mimetype = "image/jpeg"
	}
	slog.Debug("Building SendImage", "data_len", len(data), "mimetype", mimetype, "caption", caption)
	uploaded, err := ctx.Client.Upload(ctx.Ctx, data, whatsmeow.MediaImage)
	if err != nil {
		slog.Error("SendImage: upload failed", "err", err)
		return fmt.Errorf("image upload failed: %w", err)
	}
	msg := &waE2E.Message{
		ImageMessage: &waE2E.ImageMessage{
			URL:           &uploaded.URL,
			DirectPath:    &uploaded.DirectPath,
			MediaKey:      uploaded.MediaKey,
			Mimetype:      &mimetype,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    new(uint64),
			Caption:       &caption,
		},
	}
	*msg.ImageMessage.FileLength = uint64(len(data))
	slog.Info("Sending SendImage", "chat", ctx.Chat.String(), "url", uploaded.URL)
	_, err = ctx.Client.SendMessage(ctx.Ctx, ctx.Chat, msg)
	if err != nil {
		slog.Error("SendImage failed", "err", err)
	} else {
		slog.Info("SendImage sent successfully")
	}
	return err
}

// ReplyWithImage uploads and sends an image as a reply.
func (ctx *Context) ReplyWithImage(data []byte, mimetype, caption string) error {
	if mimetype == "" {
		slog.Warn("ReplyWithImage: mimetype is empty, defaulting to image/jpeg")
		mimetype = "image/jpeg"
	}
	cinfo := ctx.replyContextInfo()
	slog.Debug("Building ReplyWithImage", "data_len", len(data), "mimetype", mimetype, "caption", caption, "context_info", cinfo)
	uploaded, err := ctx.Client.Upload(ctx.Ctx, data, whatsmeow.MediaImage)
	if err != nil {
		slog.Error("ReplyWithImage: upload failed", "err", err)
		return fmt.Errorf("image upload failed: %w", err)
	}
	msg := &waE2E.Message{
		ImageMessage: &waE2E.ImageMessage{
			URL:           &uploaded.URL,
			DirectPath:    &uploaded.DirectPath,
			MediaKey:      uploaded.MediaKey,
			Mimetype:      &mimetype,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    new(uint64),
			Caption:       &caption,
			ContextInfo:   cinfo,
		},
	}
	*msg.ImageMessage.FileLength = uint64(len(data))
	slog.Info("Sending ReplyWithImage", "chat", ctx.Chat.String(), "url", uploaded.URL)
	_, err = ctx.Client.SendMessage(ctx.Ctx, ctx.Chat, msg)
	if err != nil {
		slog.Error("ReplyWithImage failed", "err", err)
	} else {
		slog.Info("ReplyWithImage sent successfully")
	}
	return err
}

// SendVideo uploads and sends a video to the current chat.
func (ctx *Context) SendVideo(data []byte, mimetype, caption string) error {
	if mimetype == "" {
		slog.Warn("SendVideo: mimetype is empty, defaulting to video/mp4")
		mimetype = "video/mp4"
	}
	slog.Debug("Building SendVideo", "data_len", len(data), "mimetype", mimetype, "caption", caption)
	uploaded, err := ctx.Client.Upload(ctx.Ctx, data, whatsmeow.MediaVideo)
	if err != nil {
		slog.Error("SendVideo: upload failed", "err", err)
		return fmt.Errorf("video upload failed: %w", err)
	}
	msg := &waE2E.Message{
		VideoMessage: &waE2E.VideoMessage{
			URL:           &uploaded.URL,
			DirectPath:    &uploaded.DirectPath,
			MediaKey:      uploaded.MediaKey,
			Mimetype:      &mimetype,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    new(uint64),
			Caption:       &caption,
		},
	}
	*msg.VideoMessage.FileLength = uint64(len(data))
	slog.Info("Sending SendVideo", "chat", ctx.Chat.String(), "url", uploaded.URL)
	_, err = ctx.Client.SendMessage(ctx.Ctx, ctx.Chat, msg)
	if err != nil {
		slog.Error("SendVideo failed", "err", err)
	} else {
		slog.Info("SendVideo sent successfully")
	}
	return err
}

// ReplyWithVideo uploads and sends a video as a reply.
func (ctx *Context) ReplyWithVideo(data []byte, mimetype, caption string) error {
	if mimetype == "" {
		slog.Warn("ReplyWithVideo: mimetype is empty, defaulting to video/mp4")
		mimetype = "video/mp4"
	}
	cinfo := ctx.replyContextInfo()
	slog.Debug("Building ReplyWithVideo", "data_len", len(data), "mimetype", mimetype, "caption", caption, "context_info", cinfo)
	uploaded, err := ctx.Client.Upload(ctx.Ctx, data, whatsmeow.MediaVideo)
	if err != nil {
		slog.Error("ReplyWithVideo: upload failed", "err", err)
		return fmt.Errorf("video upload failed: %w", err)
	}
	msg := &waE2E.Message{
		VideoMessage: &waE2E.VideoMessage{
			URL:           &uploaded.URL,
			DirectPath:    &uploaded.DirectPath,
			MediaKey:      uploaded.MediaKey,
			Mimetype:      &mimetype,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    new(uint64),
			Caption:       &caption,
			ContextInfo:   cinfo,
		},
	}
	*msg.VideoMessage.FileLength = uint64(len(data))
	slog.Info("Sending ReplyWithVideo", "chat", ctx.Chat.String(), "url", uploaded.URL)
	_, err = ctx.Client.SendMessage(ctx.Ctx, ctx.Chat, msg)
	if err != nil {
		slog.Error("ReplyWithVideo failed", "err", err)
	} else {
		slog.Info("ReplyWithVideo sent successfully")
	}
	return err
}

// SendDocument uploads and sends a document.
func (ctx *Context) SendDocument(data []byte, mimetype, filename, caption string) error {
	if mimetype == "" {
		slog.Warn("SendDocument: mimetype is empty, defaulting to application/octet-stream")
		mimetype = "application/octet-stream"
	}
	slog.Debug("Building SendDocument", "data_len", len(data), "mimetype", mimetype, "filename", filename, "caption", caption)
	uploaded, err := ctx.Client.Upload(ctx.Ctx, data, whatsmeow.MediaDocument)
	if err != nil {
		slog.Error("SendDocument: upload failed", "err", err)
		return fmt.Errorf("document upload failed: %w", err)
	}
	msg := &waE2E.Message{
		DocumentMessage: &waE2E.DocumentMessage{
			URL:           &uploaded.URL,
			DirectPath:    &uploaded.DirectPath,
			MediaKey:      uploaded.MediaKey,
			Mimetype:      &mimetype,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    new(uint64),
			FileName:      &filename,
			Caption:       &caption,
		},
	}
	*msg.DocumentMessage.FileLength = uint64(len(data))
	slog.Info("Sending SendDocument", "chat", ctx.Chat.String(), "url", uploaded.URL)
	_, err = ctx.Client.SendMessage(ctx.Ctx, ctx.Chat, msg)
	if err != nil {
		slog.Error("SendDocument failed", "err", err)
	} else {
		slog.Info("SendDocument sent successfully")
	}
	return err
}

// ReplyWithDocument uploads and sends a document as a reply.
func (ctx *Context) ReplyWithDocument(data []byte, mimetype, filename, caption string) error {
	if mimetype == "" {
		slog.Warn("ReplyWithDocument: mimetype is empty, defaulting to application/octet-stream")
		mimetype = "application/octet-stream"
	}
	cinfo := ctx.replyContextInfo()
	slog.Debug("Building ReplyWithDocument", "data_len", len(data), "mimetype", mimetype, "filename", filename, "caption", caption, "context_info", cinfo)
	uploaded, err := ctx.Client.Upload(ctx.Ctx, data, whatsmeow.MediaDocument)
	if err != nil {
		slog.Error("ReplyWithDocument: upload failed", "err", err)
		return fmt.Errorf("document upload failed: %w", err)
	}
	msg := &waE2E.Message{
		DocumentMessage: &waE2E.DocumentMessage{
			URL:           &uploaded.URL,
			DirectPath:    &uploaded.DirectPath,
			MediaKey:      uploaded.MediaKey,
			Mimetype:      &mimetype,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    new(uint64),
			FileName:      &filename,
			Caption:       &caption,
			ContextInfo:   cinfo,
		},
	}
	*msg.DocumentMessage.FileLength = uint64(len(data))
	slog.Info("Sending ReplyWithDocument", "chat", ctx.Chat.String(), "url", uploaded.URL)
	_, err = ctx.Client.SendMessage(ctx.Ctx, ctx.Chat, msg)
	if err != nil {
		slog.Error("ReplyWithDocument failed", "err", err)
	} else {
		slog.Info("ReplyWithDocument sent successfully")
	}
	return err
}

// SendSticker uploads and sends a sticker.
func (ctx *Context) SendSticker(data []byte) error {
	mimetype := "image/webp"
	slog.Debug("Building SendSticker", "data_len", len(data))
	uploaded, err := ctx.Client.Upload(ctx.Ctx, data, whatsmeow.MediaImage)
	if err != nil {
		slog.Error("SendSticker: upload failed", "err", err)
		return fmt.Errorf("sticker upload failed: %w", err)
	}
	msg := &waE2E.Message{
		StickerMessage: &waE2E.StickerMessage{
			URL:           &uploaded.URL,
			DirectPath:    &uploaded.DirectPath,
			MediaKey:      uploaded.MediaKey,
			Mimetype:      &mimetype,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    new(uint64),
		},
	}
	*msg.StickerMessage.FileLength = uint64(len(data))
	slog.Info("Sending SendSticker", "chat", ctx.Chat.String(), "url", uploaded.URL)
	_, err = ctx.Client.SendMessage(ctx.Ctx, ctx.Chat, msg)
	if err != nil {
		slog.Error("SendSticker failed", "err", err)
	} else {
		slog.Info("SendSticker sent successfully")
	}
	return err
}

// ReplyWithSticker uploads and sends a sticker as a reply.
func (ctx *Context) ReplyWithSticker(data []byte) error {
	mimetype := "image/webp"
	cinfo := ctx.replyContextInfo()
	slog.Debug("Building ReplyWithSticker", "data_len", len(data), "context_info", cinfo)
	uploaded, err := ctx.Client.Upload(ctx.Ctx, data, whatsmeow.MediaImage)
	if err != nil {
		slog.Error("ReplyWithSticker: upload failed", "err", err)
		return fmt.Errorf("sticker upload failed: %w", err)
	}
	msg := &waE2E.Message{
		StickerMessage: &waE2E.StickerMessage{
			URL:           &uploaded.URL,
			DirectPath:    &uploaded.DirectPath,
			MediaKey:      uploaded.MediaKey,
			Mimetype:      &mimetype,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    new(uint64),
			ContextInfo:   cinfo,
		},
	}
	*msg.StickerMessage.FileLength = uint64(len(data))
	slog.Info("Sending ReplyWithSticker", "chat", ctx.Chat.String(), "url", uploaded.URL)
	_, err = ctx.Client.SendMessage(ctx.Ctx, ctx.Chat, msg)
	if err != nil {
		slog.Error("ReplyWithSticker failed", "err", err)
	} else {
		slog.Info("ReplyWithSticker sent successfully")
	}
	return err
}

func (ctx *Context) getContextInfo() *waE2E.ContextInfo {
	if ctx.Evt == nil || ctx.Evt.Message == nil {
		return nil
	}
	msg := ctx.Evt.Message
	if ext := msg.GetExtendedTextMessage(); ext != nil {
		return ext.GetContextInfo()
	}
	if stk := msg.GetStickerMessage(); stk != nil {
		return stk.GetContextInfo()
	}
	if img := msg.GetImageMessage(); img != nil {
		return img.GetContextInfo()
	}
	if vid := msg.GetVideoMessage(); vid != nil {
		return vid.GetContextInfo()
	}
	if aud := msg.GetAudioMessage(); aud != nil {
		return aud.GetContextInfo()
	}
	if doc := msg.GetDocumentMessage(); doc != nil {
		return doc.GetContextInfo()
	}
	return nil
}

// GetContextInfo returns the context info of the message if available.
func (ctx *Context) GetContextInfo() *waE2E.ContextInfo {
	return ctx.getContextInfo()
}

// GetQuotedMessage returns the quoted message if this event has one.
func (ctx *Context) GetQuotedMessage() *waE2E.Message {
	ci := ctx.getContextInfo()
	if ci != nil {
		return ci.QuotedMessage
	}
	return nil
}

// GetQuotedSender returns the quoted message sender JID if available.
func (ctx *Context) GetQuotedSender() (types.JID, bool) {
	ci := ctx.getContextInfo()
	if ci != nil && ci.Participant != nil {
		pj, err := types.ParseJID(*ci.Participant)
		if err == nil {
			return pj, true
		}
	}
	return types.JID{}, false
}

// GetMentionedJIDs returns JIDs that were tagged/mentioned in the message.
func (ctx *Context) GetMentionedJIDs() []types.JID {
	ci := ctx.getContextInfo()
	if ci == nil {
		return nil
	}
	var out []types.JID
	for _, m := range ci.MentionedJID {
		j, err := types.ParseJID(m)
		if err == nil {
			out = append(out, j)
		}
	}
	return out
}

// GetArgsJIDs parses any phone numbers or JID strings in command args.
func (ctx *Context) GetArgsJIDs() []types.JID {
	var out []types.JID
	for _, arg := range ctx.Args {
		clean := strings.TrimLeft(arg, "@")
		clean = strings.TrimSpace(clean)
		if clean == "" {
			continue
		}
		if strings.Contains(clean, "@") {
			j, err := types.ParseJID(clean)
			if err == nil {
				out = append(out, j)
			}
		} else {
			var digits strings.Builder
			for _, r := range clean {
				if r >= '0' && r <= '9' {
					digits.WriteRune(r)
				}
			}
			if digits.Len() > 0 {
				j, err := types.ParseJID(digits.String() + "@" + types.DefaultUserServer)
				if err == nil {
					out = append(out, j)
				}
			}
		}
	}
	return out
}

// IsSameUserRaw compares two JIDs, resolving and matching any LID mappings.
func IsSameUserRaw(ctx context.Context, client *whatsmeow.Client, a, b types.JID) bool {
	slog.Debug("IsSameUserRaw checking", "a", a.String(), "b", b.String())
	a = a.ToNonAD()
	b = b.ToNonAD()
	if a == b {
		slog.Debug("IsSameUserRaw result: true (direct match)", "a", a.String(), "b", b.String())
		return true
	}

	aPN := a
	bPN := b

	if a.Server == types.HiddenUserServer && client.Store.LIDs != nil {
		if pn, err := client.Store.LIDs.GetPNForLID(ctx, a); err == nil && !pn.IsEmpty() {
			aPN = pn.ToNonAD()
		}
	}
	if b.Server == types.HiddenUserServer && client.Store.LIDs != nil {
		if pn, err := client.Store.LIDs.GetPNForLID(ctx, b); err == nil && !pn.IsEmpty() {
			bPN = pn.ToNonAD()
		}
	}

	if aPN == bPN {
		slog.Debug("IsSameUserRaw result: true (PN match)", "a", a.String(), "b", b.String())
		return true
	}

	aLID := a
	bLID := b
	if a.Server == types.DefaultUserServer && client.Store.LIDs != nil {
		if lid, err := client.Store.LIDs.GetLIDForPN(ctx, a); err == nil && !lid.IsEmpty() {
			aLID = lid.ToNonAD()
		}
	}
	if b.Server == types.DefaultUserServer && client.Store.LIDs != nil {
		if lid, err := client.Store.LIDs.GetLIDForPN(ctx, b); err == nil && !lid.IsEmpty() {
			bLID = lid.ToNonAD()
		}
	}

	res := aLID == bLID
	slog.Debug("IsSameUserRaw result", "a", a.String(), "b", b.String(), "result", res)
	return res
}

// IsSameUser compares two JIDs, resolving and matching any LID mappings.
func (ctx *Context) IsSameUser(a, b types.JID) bool {
	res := IsSameUserRaw(ctx.Ctx, ctx.Client, a, b)
	slog.Debug("IsSameUser helper check", "a", a.String(), "b", b.String(), "result", res)
	return res
}

// GetTargets resolves targets from reply, mentions, or arguments.
// If in a P2P chat and no other target is provided (or if the provided target/sender is ours),
// we fall back to the chat JID (as long as it isn't ours).
func (ctx *Context) GetTargets() []types.JID {
	if ctx.Client.Store.ID == nil {
		return nil
	}
	ourJID := *ctx.Client.Store.ID

	// 1. Quoted message sender (excluding ours)
	if q, ok := ctx.GetQuotedSender(); ok {
		if !ctx.IsSameUser(q, ourJID) {
			return []types.JID{q}
		}
	}

	// 2. Mentioned JIDs (excluding ours)
	if m := ctx.GetMentionedJIDs(); len(m) > 0 {
		var filtered []types.JID
		for _, j := range m {
			if !ctx.IsSameUser(j, ourJID) {
				filtered = append(filtered, j)
			}
		}
		if len(filtered) > 0 {
			return filtered
		}
	}

	// 3. Arguments JIDs (excluding ours)
	if argsJIDs := ctx.GetArgsJIDs(); len(argsJIDs) > 0 {
		var filtered []types.JID
		for _, j := range argsJIDs {
			if !ctx.IsSameUser(j, ourJID) {
				filtered = append(filtered, j)
			}
		}
		if len(filtered) > 0 {
			return filtered
		}
	}

	// 4. In a P2P chat (chat server is not g.us) and the chat JID is not ours
	if ctx.Chat.Server != "g.us" {
		if !ctx.IsSameUser(ctx.Chat, ourJID) {
			return []types.JID{ctx.Chat}
		}
	}

	return nil
}

// IsOwner checks if the message sender is the bot owner (the connected WhatsApp account JID).
func (ctx *Context) IsOwner() bool {
	if ctx.Client.Store.ID != nil {
		return ctx.IsSameUser(ctx.Sender, *ctx.Client.Store.ID)
	}
	return false
}

// IsSudo checks if the message sender is a registered sudo user or the bot owner.
func (ctx *Context) IsSudo() bool {
	slog.Debug("IsSudo checking", "sender", ctx.Sender.String())
	if ctx.IsOwner() {
		slog.Debug("IsSudo result: true (bot owner)", "sender", ctx.Sender.String())
		return true
	}

	s, ok := ctx.Client.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		slog.Debug("IsSudo result: false (settings store unavailable)", "sender", ctx.Sender.String())
		return false
	}
	raw, err := s.GetSetting(ctx.Ctx, "sudoers")
	if err != nil || raw == "" {
		slog.Debug("IsSudo result: false (no sudoers configured)", "sender", ctx.Sender.String())
		return false
	}

	for sudoerStr := range strings.FieldsSeq(raw) {
		sudoerJID, err := types.ParseJID(sudoerStr)
		if err == nil {
			if ctx.IsSameUser(ctx.Sender, sudoerJID) {
				slog.Debug("IsSudo result: true (sudoer list match)", "sender", ctx.Sender.String())
				return true
			}
		}
	}
	slog.Debug("IsSudo result: false", "sender", ctx.Sender.String())
	return false
}

// GetMedia retrieves media bytes and mimetype from the message or its quoted message.
func (ctx *Context) GetMedia() ([]byte, string, error) {
	// First check the main message
	if data, mimetype, ok := ctx.extractMediaFromMessage(ctx.Evt.Message); ok {
		return data, mimetype, nil
	}
	// Then check quoted message
	if quoted := ctx.GetQuotedMessage(); quoted != nil {
		if data, mimetype, ok := ctx.extractMediaFromMessage(quoted); ok {
			return data, mimetype, nil
		}
	}
	return nil, "", fmt.Errorf("no media found in message or quoted message")
}

func (ctx *Context) extractMediaFromMessage(msg *waE2E.Message) ([]byte, string, bool) {
	if msg == nil {
		return nil, "", false
	}
	var downloadable whatsmeow.DownloadableMessage
	var mimetype string

	if img := msg.GetImageMessage(); img != nil {
		downloadable = img
		mimetype = img.GetMimetype()
	} else if vid := msg.GetVideoMessage(); vid != nil {
		downloadable = vid
		mimetype = vid.GetMimetype()
	} else if aud := msg.GetAudioMessage(); aud != nil {
		downloadable = aud
		mimetype = aud.GetMimetype()
	} else if doc := msg.GetDocumentMessage(); doc != nil {
		downloadable = doc
		mimetype = doc.GetMimetype()
	} else if stk := msg.GetStickerMessage(); stk != nil {
		downloadable = stk
		mimetype = stk.GetMimetype()
	}

	if downloadable == nil {
		return nil, "", false
	}

	data, err := ctx.Client.Download(ctx.Ctx, downloadable)
	if err != nil {
		return nil, "", false
	}
	if mimetype == "" {
		if msg.GetImageMessage() != nil {
			mimetype = "image/jpeg"
		} else if msg.GetVideoMessage() != nil {
			mimetype = "video/mp4"
		} else if msg.GetAudioMessage() != nil {
			mimetype = "audio/ogg"
		} else if msg.GetDocumentMessage() != nil {
			mimetype = "application/octet-stream"
		} else if msg.GetStickerMessage() != nil {
			mimetype = "image/webp"
		}
	}
	return data, mimetype, true
}

// SendAudio uploads and sends an audio file (with recording simulation).
func (ctx *Context) SendAudio(data []byte, mimetype string) error {
	ctx.simulateRecording()
	if mimetype == "" {
		slog.Warn("SendAudio: mimetype is empty, defaulting to audio/mp4")
		mimetype = "audio/mp4"
	}
	slog.Debug("Building SendAudio", "data_len", len(data), "mimetype", mimetype)
	uploaded, err := ctx.Client.Upload(ctx.Ctx, data, whatsmeow.MediaAudio)
	if err != nil {
		slog.Error("SendAudio: upload failed", "err", err)
		return fmt.Errorf("audio upload failed: %w", err)
	}
	msg := &waE2E.Message{
		AudioMessage: &waE2E.AudioMessage{
			URL:           &uploaded.URL,
			DirectPath:    &uploaded.DirectPath,
			MediaKey:      uploaded.MediaKey,
			Mimetype:      &mimetype,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    new(uint64),
		},
	}
	*msg.AudioMessage.FileLength = uint64(len(data))
	slog.Info("Sending SendAudio", "chat", ctx.Chat.String(), "url", uploaded.URL)
	_, err = ctx.Client.SendMessage(ctx.Ctx, ctx.Chat, msg)
	if err != nil {
		slog.Error("SendAudio failed", "err", err)
	} else {
		slog.Info("SendAudio sent successfully")
	}
	return err
}

// ReplyWithAudio uploads and sends an audio file as a reply (with recording simulation).
func (ctx *Context) ReplyWithAudio(data []byte, mimetype string) error {
	ctx.simulateRecording()
	if mimetype == "" {
		slog.Warn("ReplyWithAudio: mimetype is empty, defaulting to audio/mp4")
		mimetype = "audio/mp4"
	}
	cinfo := ctx.replyContextInfo()
	slog.Debug("Building ReplyWithAudio", "data_len", len(data), "mimetype", mimetype, "context_info", cinfo)
	uploaded, err := ctx.Client.Upload(ctx.Ctx, data, whatsmeow.MediaAudio)
	if err != nil {
		slog.Error("ReplyWithAudio: upload failed", "err", err)
		return fmt.Errorf("audio upload failed: %w", err)
	}
	msg := &waE2E.Message{
		AudioMessage: &waE2E.AudioMessage{
			URL:           &uploaded.URL,
			DirectPath:    &uploaded.DirectPath,
			MediaKey:      uploaded.MediaKey,
			Mimetype:      &mimetype,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    new(uint64),
			ContextInfo:   cinfo,
		},
	}
	*msg.AudioMessage.FileLength = uint64(len(data))
	slog.Info("Sending ReplyWithAudio", "chat", ctx.Chat.String(), "url", uploaded.URL)
	_, err = ctx.Client.SendMessage(ctx.Ctx, ctx.Chat, msg)
	if err != nil {
		slog.Error("ReplyWithAudio failed", "err", err)
	} else {
		slog.Info("ReplyWithAudio sent successfully")
	}
	return err
}

// SendTextWithMentions sends a text message with WhatsApp mentions.
func (ctx *Context) SendTextWithMentions(text string, jids []types.JID) error {
	ctx.simulateTyping()
	formatted := ctx.formatMentionTextResponse(text)
	var mentioned []string
	for _, j := range jids {
		mentioned = append(mentioned, j.ToNonAD().String())
	}
	slog.Debug("Building SendTextWithMentions", "text", text, "formatted", formatted, "mentioned_jids", mentioned)
	slog.Info("Sending SendTextWithMentions", "chat", ctx.Chat.String())
	_, err := ctx.Client.SendMessage(ctx.Ctx, ctx.Chat, &waE2E.Message{
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text: &formatted,
			ContextInfo: &waE2E.ContextInfo{
				MentionedJID: mentioned,
			},
		},
	})
	if err != nil {
		slog.Error("SendTextWithMentions failed", "err", err)
	} else {
		slog.Info("SendTextWithMentions sent successfully")
	}
	return err
}

// ReplyWithMentions sends a text message with WhatsApp mentions replying to the current message.
func (ctx *Context) ReplyWithMentions(text string, jids []types.JID) error {
	ctx.simulateTyping()
	formatted := ctx.formatMentionTextResponse(text)
	var mentioned []string
	for _, j := range jids {
		mentioned = append(mentioned, j.ToNonAD().String())
	}
	cInfo := ctx.replyContextInfo()
	if cInfo != nil {
		cInfo.MentionedJID = mentioned
	} else {
		cInfo = &waE2E.ContextInfo{
			MentionedJID: mentioned,
		}
	}
	slog.Debug("Building ReplyWithMentions", "text", text, "formatted", formatted, "mentioned_jids", mentioned, "context_info", cInfo)
	slog.Info("Sending ReplyWithMentions", "chat", ctx.Chat.String())
	_, err := ctx.Client.SendMessage(ctx.Ctx, ctx.Chat, &waE2E.Message{
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text:        &formatted,
			ContextInfo: cInfo,
		},
	})
	if err != nil {
		slog.Error("ReplyWithMentions failed", "err", err)
	} else {
		slog.Info("ReplyWithMentions sent successfully")
	}
	return err
}

// ResolveMentionRaw returns the resolved JID and username matching display representation for mentions.
func ResolveMentionRaw(ctx context.Context, client *whatsmeow.Client, jid types.JID) (types.JID, string) {
	jid = jid.ToNonAD()
	if jid.Server == types.HiddenUserServer && client.Store.LIDs != nil {
		if pn, err := client.Store.LIDs.GetPNForLID(ctx, jid); err == nil && !pn.IsEmpty() {
			return pn.ToNonAD(), pn.User
		}
	}
	return jid, jid.User
}

// ResolveMention returns the resolved JID and username matching display representation for mentions.
func (ctx *Context) ResolveMention(jid types.JID) (types.JID, string) {
	return ResolveMentionRaw(ctx.Ctx, ctx.Client, jid)
}

func (ctx *Context) formatMentionTextResponse(text string) string {
	text = strings.ReplaceAll(text, "*", "")
	text = removeEmojis(text)
	return text
}

// Protobuf Helper: encodeProtoMessage marshal and base64 encodes a message
func EncodeProtoMessage(msg *waE2E.Message) (string, error) {
	bytes, err := proto.Marshal(msg)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(bytes), nil
}

// Protobuf Helper: decodeProtoMessage base64 decodes and unmarshals a message
func DecodeProtoMessage(encoded string) (*waE2E.Message, error) {
	bytes, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, err
	}
	var msg waE2E.Message
	err = proto.Unmarshal(bytes, &msg)
	if err != nil {
		return nil, err
	}
	return &msg, nil
}

// ExtractViewOnceMessage unwraps any ViewOnce message container and clears the ViewOnce flag.
func ExtractViewOnceMessage(msg *waE2E.Message) *waE2E.Message {
	if msg == nil {
		return nil
	}

	var inner *waE2E.Message
	if msg.ViewOnceMessage != nil && msg.ViewOnceMessage.Message != nil {
		inner = msg.ViewOnceMessage.Message
	} else if msg.ViewOnceMessageV2 != nil && msg.ViewOnceMessageV2.Message != nil {
		inner = msg.ViewOnceMessageV2.Message
	} else if msg.ViewOnceMessageV2Extension != nil && msg.ViewOnceMessageV2Extension.Message != nil {
		inner = msg.ViewOnceMessageV2Extension.Message
	} else {
		inner = msg
	}

	cloned := proto.Clone(inner).(*waE2E.Message)

	if cloned.ImageMessage != nil {
		cloned.ImageMessage.ViewOnce = new(bool)
	}
	if cloned.VideoMessage != nil {
		cloned.VideoMessage.ViewOnce = new(bool)
	}
	if cloned.AudioMessage != nil {
		cloned.AudioMessage.ViewOnce = new(bool)
	}

	return cloned
}

// IsAdminRaw checks if a specific JID is a group admin.
func IsAdminRaw(ctx context.Context, client *whatsmeow.Client, info *types.GroupInfo, jid types.JID) bool {
	slog.Debug("IsAdminRaw checking", "jid", jid.String(), "group", info.JID.String())
	target := jid.ToNonAD()
	for _, p := range info.Participants {
		if IsSameUserRaw(ctx, client, p.JID, target) {
			res := p.IsAdmin || p.IsSuperAdmin
			slog.Debug("IsAdminRaw result", "jid", jid.String(), "isAdmin", res, "isSuperAdmin", p.IsSuperAdmin)
			return res
		}
	}
	slog.Debug("IsAdminRaw result: false (not a participant)", "jid", jid.String())
	return false
}

// IsAdmin checks if a specific JID is a group admin.
func (ctx *Context) IsAdmin(info *types.GroupInfo, jid types.JID) bool {
	res := IsAdminRaw(ctx.Ctx, ctx.Client, info, jid)
	slog.Debug("IsAdmin helper check", "jid", jid.String(), "result", res)
	return res
}

// AmIAdmin checks if the bot itself is an admin in the group.
func (ctx *Context) AmIAdmin(info *types.GroupInfo) bool {
	slog.Debug("AmIAdmin checking")
	if ctx.Client.Store.ID == nil {
		slog.Debug("AmIAdmin result: false (bot JID nil)")
		return false
	}
	res := ctx.IsAdmin(info, *ctx.Client.Store.ID)
	slog.Debug("AmIAdmin result", "result", res)
	return res
}

// IsSenderAdmin checks if the command sender is a group admin or bot sudoer.
func (ctx *Context) IsSenderAdmin(info *types.GroupInfo) bool {
	slog.Debug("IsSenderAdmin checking", "sender", ctx.Sender.String())
	if ctx.IsSudo() {
		slog.Debug("IsSenderAdmin result: true (is sudo)")
		return true
	}
	res := ctx.IsAdmin(info, ctx.Sender)
	slog.Debug("IsSenderAdmin result", "sender", ctx.Sender.String(), "result", res)
	return res
}

func removeEmojis(s string) string {
	var sb strings.Builder
	for _, r := range s {
		if (r >= 0x1F000 && r <= 0x1F9FF) || (r >= 0x2600 && r <= 0x27BF) || (r >= 0x1FA00 && r <= 0x1FAFF) || (r >= 0x1F1E0 && r <= 0x1F1FF) {
			continue // skip emoji
		}
		sb.WriteRune(r)
	}
	return sb.String()
}
