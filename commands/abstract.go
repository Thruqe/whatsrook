package commands

import (
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/Thruqe/whatsrook/store/sqlstore"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

// formatTextResponse strips asterisks, emojis, and wraps response in 3 backticks for monospace formatting
func (ctx *Context) formatTextResponse(text string) string {
	text = strings.ReplaceAll(text, "*", "")
	text = removeEmojis(text)
	return "```\n" + text + "\n```"
}

// simulateTyping simulates text typing presence for 3 seconds
func (ctx *Context) simulateTyping() {
	_ = ctx.Client.SendChatPresence(ctx.Ctx, ctx.Chat, types.ChatPresenceComposing, types.ChatPresenceMediaText)
	time.Sleep(3 * time.Second)
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
	_, err := ctx.Client.SendMessage(ctx.Ctx, ctx.Chat, &waE2E.Message{
		Conversation: &formatted,
	})
	return err
}

// Reply sends a text message replying to the current message (with typing simulation and monospace format).
func (ctx *Context) Reply(text string) error {
	ctx.simulateTyping()
	formatted := ctx.formatTextResponse(text)
	_, err := ctx.Client.SendMessage(ctx.Ctx, ctx.Chat, &waE2E.Message{
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text:        &formatted,
			ContextInfo: ctx.replyContextInfo(),
		},
	})
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
		mimetype = "image/jpeg"
	}
	uploaded, err := ctx.Client.Upload(ctx.Ctx, data, whatsmeow.MediaImage)
	if err != nil {
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
	_, err = ctx.Client.SendMessage(ctx.Ctx, ctx.Chat, msg)
	return err
}

// ReplyWithImage uploads and sends an image as a reply.
func (ctx *Context) ReplyWithImage(data []byte, mimetype, caption string) error {
	if mimetype == "" {
		mimetype = "image/jpeg"
	}
	uploaded, err := ctx.Client.Upload(ctx.Ctx, data, whatsmeow.MediaImage)
	if err != nil {
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
			ContextInfo:   ctx.replyContextInfo(),
		},
	}
	*msg.ImageMessage.FileLength = uint64(len(data))
	_, err = ctx.Client.SendMessage(ctx.Ctx, ctx.Chat, msg)
	return err
}

// SendVideo uploads and sends a video to the current chat.
func (ctx *Context) SendVideo(data []byte, mimetype, caption string) error {
	if mimetype == "" {
		mimetype = "video/mp4"
	}
	uploaded, err := ctx.Client.Upload(ctx.Ctx, data, whatsmeow.MediaVideo)
	if err != nil {
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
	_, err = ctx.Client.SendMessage(ctx.Ctx, ctx.Chat, msg)
	return err
}

// ReplyWithVideo uploads and sends a video as a reply.
func (ctx *Context) ReplyWithVideo(data []byte, mimetype, caption string) error {
	if mimetype == "" {
		mimetype = "video/mp4"
	}
	uploaded, err := ctx.Client.Upload(ctx.Ctx, data, whatsmeow.MediaVideo)
	if err != nil {
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
			ContextInfo:   ctx.replyContextInfo(),
		},
	}
	*msg.VideoMessage.FileLength = uint64(len(data))
	_, err = ctx.Client.SendMessage(ctx.Ctx, ctx.Chat, msg)
	return err
}

// SendDocument uploads and sends a document.
func (ctx *Context) SendDocument(data []byte, mimetype, filename, caption string) error {
	if mimetype == "" {
		mimetype = "application/octet-stream"
	}
	uploaded, err := ctx.Client.Upload(ctx.Ctx, data, whatsmeow.MediaDocument)
	if err != nil {
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
	_, err = ctx.Client.SendMessage(ctx.Ctx, ctx.Chat, msg)
	return err
}

// ReplyWithDocument uploads and sends a document as a reply.
func (ctx *Context) ReplyWithDocument(data []byte, mimetype, filename, caption string) error {
	if mimetype == "" {
		mimetype = "application/octet-stream"
	}
	uploaded, err := ctx.Client.Upload(ctx.Ctx, data, whatsmeow.MediaDocument)
	if err != nil {
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
			ContextInfo:   ctx.replyContextInfo(),
		},
	}
	*msg.DocumentMessage.FileLength = uint64(len(data))
	_, err = ctx.Client.SendMessage(ctx.Ctx, ctx.Chat, msg)
	return err
}

// SendSticker uploads and sends a sticker.
func (ctx *Context) SendSticker(data []byte) error {
	mimetype := "image/webp"
	uploaded, err := ctx.Client.Upload(ctx.Ctx, data, whatsmeow.MediaImage)
	if err != nil {
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
	_, err = ctx.Client.SendMessage(ctx.Ctx, ctx.Chat, msg)
	return err
}

// ReplyWithSticker uploads and sends a sticker as a reply.
func (ctx *Context) ReplyWithSticker(data []byte) error {
	mimetype := "image/webp"
	uploaded, err := ctx.Client.Upload(ctx.Ctx, data, whatsmeow.MediaImage)
	if err != nil {
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
			ContextInfo:   ctx.replyContextInfo(),
		},
	}
	*msg.StickerMessage.FileLength = uint64(len(data))
	_, err = ctx.Client.SendMessage(ctx.Ctx, ctx.Chat, msg)
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

// IsSameUser compares two JIDs, resolving and matching any LID mappings.
func (ctx *Context) IsSameUser(a, b types.JID) bool {
	a = a.ToNonAD()
	b = b.ToNonAD()
	if a == b {
		return true
	}

	aPN := a
	bPN := b

	if a.Server == types.HiddenUserServer && ctx.Client.Store.LIDs != nil {
		if pn, err := ctx.Client.Store.LIDs.GetPNForLID(ctx.Ctx, a); err == nil && !pn.IsEmpty() {
			aPN = pn.ToNonAD()
		}
	}
	if b.Server == types.HiddenUserServer && ctx.Client.Store.LIDs != nil {
		if pn, err := ctx.Client.Store.LIDs.GetPNForLID(ctx.Ctx, b); err == nil && !pn.IsEmpty() {
			bPN = pn.ToNonAD()
		}
	}

	if aPN == bPN {
		return true
	}

	aLID := a
	bLID := b
	if a.Server == types.DefaultUserServer && ctx.Client.Store.LIDs != nil {
		if lid, err := ctx.Client.Store.LIDs.GetLIDForPN(ctx.Ctx, a); err == nil && !lid.IsEmpty() {
			aLID = lid.ToNonAD()
		}
	}
	if b.Server == types.DefaultUserServer && ctx.Client.Store.LIDs != nil {
		if lid, err := ctx.Client.Store.LIDs.GetLIDForPN(ctx.Ctx, b); err == nil && !lid.IsEmpty() {
			bLID = lid.ToNonAD()
		}
	}

	return aLID == bLID
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

// IsSudo checks if the message sender is a registered sudo user or the bot owner.
func (ctx *Context) IsSudo() bool {
	if ctx.Client.Store.ID != nil {
		if ctx.IsSameUser(ctx.Sender, *ctx.Client.Store.ID) {
			return true
		}
	}

	s, ok := ctx.Client.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return false
	}
	raw, err := s.GetSetting(ctx.Ctx, "sudoers")
	if err != nil || raw == "" {
		return false
	}

	for _, sudoerStr := range strings.Fields(raw) {
		sudoerJID, err := types.ParseJID(sudoerStr)
		if err == nil {
			if ctx.IsSameUser(ctx.Sender, sudoerJID) {
				return true
			}
		}
	}
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
		mimetype = "audio/mp4"
	}
	uploaded, err := ctx.Client.Upload(ctx.Ctx, data, whatsmeow.MediaAudio)
	if err != nil {
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
	_, err = ctx.Client.SendMessage(ctx.Ctx, ctx.Chat, msg)
	return err
}

// ReplyWithAudio uploads and sends an audio file as a reply (with recording simulation).
func (ctx *Context) ReplyWithAudio(data []byte, mimetype string) error {
	ctx.simulateRecording()
	if mimetype == "" {
		mimetype = "audio/mp4"
	}
	uploaded, err := ctx.Client.Upload(ctx.Ctx, data, whatsmeow.MediaAudio)
	if err != nil {
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
			ContextInfo:   ctx.replyContextInfo(),
		},
	}
	*msg.AudioMessage.FileLength = uint64(len(data))
	_, err = ctx.Client.SendMessage(ctx.Ctx, ctx.Chat, msg)
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
	_, err := ctx.Client.SendMessage(ctx.Ctx, ctx.Chat, &waE2E.Message{
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text: &formatted,
			ContextInfo: &waE2E.ContextInfo{
				MentionedJID: mentioned,
			},
		},
	})
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
	_, err := ctx.Client.SendMessage(ctx.Ctx, ctx.Chat, &waE2E.Message{
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text:        &formatted,
			ContextInfo: cInfo,
		},
	})
	return err
}

// ResolveMention returns the resolved JID and username matching display representation for mentions.
func (ctx *Context) ResolveMention(jid types.JID) (types.JID, string) {
	jid = jid.ToNonAD()
	if jid.Server == types.HiddenUserServer && ctx.Client.Store.LIDs != nil {
		if pn, err := ctx.Client.Store.LIDs.GetPNForLID(ctx.Ctx, jid); err == nil && !pn.IsEmpty() {
			return pn.ToNonAD(), pn.User
		}
	}
	return jid, jid.User
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

	res := *inner

	if res.ImageMessage != nil {
		res.ImageMessage.ViewOnce = new(bool)
	}
	if res.VideoMessage != nil {
		res.VideoMessage.ViewOnce = new(bool)
	}
	if res.AudioMessage != nil {
		res.AudioMessage.ViewOnce = new(bool)
	}

	return &res
}
