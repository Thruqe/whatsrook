// Package utils provides common helper routines for message parsing, fonts, and event payload formatting.
package utils

import (
	"whatsrook/wa-core/proto/waE2E"
	"whatsrook/wa-core/types/events"
)

// ExtractTextFromProto extracts conversation text, extended text, or media caption from a waE2E.Message proto.
func ExtractTextFromProto(msg *waE2E.Message) string {
	if msg == nil {
		return ""
	}
	if msg.GetConversation() != "" {
		return msg.GetConversation()
	}
	if ext := msg.GetExtendedTextMessage(); ext != nil {
		return ext.GetText()
	}
	if doc := msg.GetDocumentMessage(); doc != nil {
		return doc.GetCaption()
	}
	if img := msg.GetImageMessage(); img != nil {
		return img.GetCaption()
	}
	if vid := msg.GetVideoMessage(); vid != nil {
		return vid.GetCaption()
	}
	return ""
}

// ExtractMessageText extracts the primary text content from a whatsmeow *events.Message.
func ExtractMessageText(v *events.Message) string {
	if v == nil || v.Message == nil {
		return ""
	}
	if v.Message.GetConversation() != "" {
		return v.Message.GetConversation()
	}
	if v.Message.GetExtendedTextMessage() != nil {
		return v.Message.GetExtendedTextMessage().GetText()
	}
	if v.Message.DocumentMessage.GetCaption() != "" {
		return v.Message.DocumentMessage.GetCaption()
	}
	if v.Message.ImageMessage.GetCaption() != "" {
		return v.Message.ImageMessage.GetCaption()
	}
	if v.Message.VideoMessage.GetCaption() != "" {
		return v.Message.VideoMessage.GetCaption()
	}
	return ""
}

// GetMediaType returns the simple media string ("image", "video", "audio", "document", "sticker", "contact", "location") from a waE2E.Message.
func GetMediaType(msg *waE2E.Message) string {
	if msg == nil {
		return ""
	}
	switch {
	case msg.ImageMessage != nil:
		return "image"
	case msg.VideoMessage != nil:
		return "video"
	case msg.AudioMessage != nil:
		return "audio"
	case msg.DocumentMessage != nil:
		return "document"
	case msg.StickerMessage != nil:
		return "sticker"
	case msg.ContactMessage != nil || msg.ContactsArrayMessage != nil:
		return "contact"
	case msg.LocationMessage != nil || msg.LiveLocationMessage != nil:
		return "location"
	default:
		return ""
	}
}
