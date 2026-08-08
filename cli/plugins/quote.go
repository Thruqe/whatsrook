// Quote command – generates a stylized message bubble sticker from user text or replied message.
package plugins

import (
	"bytes"
	"image/png"
	"log/slog"
	"os"
	"strings"
	"time"

	"whatsrook/utils"
	"whatsrook/wa-core"
	"whatsrook/wa-core/types"
)

func init() {
	Register(&Command{
		Name:        "quote",
		Aliases:     []string{"q"},
		Description: "Create a WhatsApp-style quote sticker from text or a replied message",
		Category:    "media",
		IsPublic:    true,
		Handler:     handleQuote,
	})
}

func handleQuote(ctx *Context) error {
	loader := ctx.StartLoader("Generating quote sticker...")
	defer loader.Delete()

	var msg utils.QuoteMessage
	scheme := utils.DefaultQuoteScheme()

	quotedMsg := ctx.GetQuotedMessage()
	quotedSenderJID, hasQuotedSender := ctx.GetQuotedSender()

	slog.Info("handleQuote: Debugging incoming quote request",
		"args_len", len(ctx.Args),
		"has_quoted_msg", quotedMsg != nil,
		"has_quoted_sender", hasQuotedSender,
		"quoted_sender_jid", quotedSenderJID.String(),
		"chat_jid", ctx.Chat.String(),
		"sender_jid", ctx.Sender.String(),
	)

	// Determine target message & target user
	if quotedMsg != nil && hasQuotedSender {
		// Case 1: User replied to a message — quote the REPLIED message and user!
		repliedText := extractTextFromProto(quotedMsg)
		slog.Info("handleQuote: Quoted message found", "replied_text", repliedText)

		if repliedText == "" {
			return ctx.Reply("The replied message contains no text.")
		}

		rawTargetJID := quotedSenderJID
		targetPNJID := NormalizeUserJID(ctx.Ctx, ctx.Client, rawTargetJID)
		targetPushName := resolveUserPushName(ctx, targetPNJID, rawTargetJID)

		msg.Username = targetPushName
		msg.UserPhone = targetPNJID.User
		msg.Content = repliedText
		msg.AvatarPath = fetchAvatarPath(ctx, targetPNJID, rawTargetJID)
		if !ctx.Evt.Info.Timestamp.IsZero() {
			msg.Timestamp = ctx.Evt.Info.Timestamp.Format("15:04")
		} else {
			msg.Timestamp = time.Now().Format("15:04")
		}

		slog.Info("handleQuote: Target resolved for replied message",
			"username", msg.Username,
			"phone", msg.UserPhone,
			"avatar", msg.AvatarPath,
		)

		// Check nested quote (if replied user's message itself quotes another message)
		if innerCI := getContextInfoFromProto(quotedMsg); innerCI != nil && innerCI.QuotedMessage != nil {
			nestedText := extractTextFromProto(innerCI.QuotedMessage)
			slog.Info("handleQuote: Inner nested quote checked", "nested_text", nestedText)

			if nestedText != "" {
				msg.Quoted = true
				msg.QuotedText = nestedText
				nestedPushName := "Quoted User"

				var nestedRawJID types.JID
				if innerCI.Participant != nil && *innerCI.Participant != "" {
					if pj, err := types.ParseJID(*innerCI.Participant); err == nil {
						nestedRawJID = pj
					}
				} else if innerCI.RemoteJID != nil && *innerCI.RemoteJID != "" {
					if pj, err := types.ParseJID(*innerCI.RemoteJID); err == nil {
						nestedRawJID = pj
					}
				}

				if !nestedRawJID.IsEmpty() {
					nestedPNJID := NormalizeUserJID(ctx.Ctx, ctx.Client, nestedRawJID)
					nestedPushName = resolveUserPushName(ctx, nestedPNJID, nestedRawJID)
				}
				msg.QuotedName = nestedPushName
			} else {
				msg.Quoted = false
			}
		} else {
			msg.Quoted = false
		}

	} else if len(ctx.Args) > 0 {
		// Case 2: User typed `.quote <text>` directly (no reply) — quote command initiator!
		senderPNJID := NormalizeUserJID(ctx.Ctx, ctx.Client, ctx.Sender)
		senderPushName := resolveUserPushName(ctx, senderPNJID, ctx.Sender)

		msg.Username = senderPushName
		msg.UserPhone = senderPNJID.User
		msg.Content = strings.TrimSpace(ctx.RawArgs)
		msg.AvatarPath = fetchAvatarPath(ctx, senderPNJID, ctx.Sender)
		if !ctx.Evt.Info.Timestamp.IsZero() {
			msg.Timestamp = ctx.Evt.Info.Timestamp.Format("15:04")
		} else {
			msg.Timestamp = time.Now().Format("15:04")
		}

		slog.Info("handleQuote: Direct text quote for sender",
			"username", msg.Username,
			"phone", msg.UserPhone,
			"avatar", msg.AvatarPath,
		)
	} else {
		return ctx.Reply("Usage: `.quote <text>` or reply to a text message with `.quote`.")
	}

	// Render image
	img, err := utils.RenderQuote(ctx.Ctx, msg, scheme)
	if err != nil {
		slog.Error("handleQuote: RenderQuote failed", "err", err)
		return ctx.Reply("Failed to render quote image: " + err.Error())
	}

	buf := &bytes.Buffer{}
	if err := png.Encode(buf, img); err != nil {
		return ctx.Reply("Failed to encode image: " + err.Error())
	}

	packName := ctx.GetBotName()
	author := "WhatsRook Quote"
	stickerData, err := processSticker(buf.Bytes(), false, packName, author, "")
	if err != nil {
		slog.Error("handleQuote: processSticker failed", "err", err)
		return ctx.Reply("Failed to convert quote image to sticker: " + err.Error())
	}

	return ctx.ReplyWithSticker(stickerData)
}

func resolveUserPushName(ctx *Context, pnjid types.JID, rawJID types.JID) string {
	// If event sender is rawJID, check PushName from event info
	if !rawJID.IsEmpty() && ctx.Evt != nil && ctx.Evt.Info.Sender.ToNonAD().User == rawJID.ToNonAD().User && ctx.Evt.Info.PushName != "" {
		return ctx.Evt.Info.PushName
	}

	if ctx.Client != nil && ctx.Client.Store != nil && ctx.Client.Store.Contacts != nil {
		// Check PN contact
		if contact, err := ctx.Client.Store.Contacts.GetContact(ctx.Ctx, pnjid); err == nil && contact.Found {
			if contact.PushName != "" {
				return contact.PushName
			}
			if contact.FullName != "" {
				return contact.FullName
			}
			if contact.BusinessName != "" {
				return contact.BusinessName
			}
		}
		// Check raw LID contact if different
		if rawJID != pnjid && !rawJID.IsEmpty() {
			if contact, err := ctx.Client.Store.Contacts.GetContact(ctx.Ctx, rawJID); err == nil && contact.Found {
				if contact.PushName != "" {
					return contact.PushName
				}
				if contact.FullName != "" {
					return contact.FullName
				}
				if contact.BusinessName != "" {
					return contact.BusinessName
				}
			}
		}
	}

	if pnjid.User != "" {
		return pnjid.User
	}
	return "User"
}

func fetchAvatarPath(ctx *Context, pnjid types.JID, rawJID types.JID) string {
	if ctx.Client == nil {
		slog.Warn("fetchAvatarPath: client is nil")
		return ""
	}

	queryJIDs := []types.JID{pnjid}
	if !rawJID.IsEmpty() && rawJID != pnjid {
		queryJIDs = append(queryJIDs, rawJID)
	}

	for _, target := range queryJIDs {
		slog.Info("fetchAvatarPath: Requesting profile picture info", "target_jid", target.String())
		ppInfo, err := ctx.Client.GetProfilePictureInfo(ctx.Ctx, target, &whatsmeow.GetProfilePictureParams{})
		if err != nil {
			slog.Warn("fetchAvatarPath: GetProfilePictureInfo error", "target_jid", target.String(), "err", err)
			continue
		}
		if ppInfo == nil || ppInfo.URL == "" {
			slog.Warn("fetchAvatarPath: GetProfilePictureInfo returned empty URL", "target_jid", target.String())
			continue
		}

		slog.Info("fetchAvatarPath: Profile picture URL fetched successfully", "target_jid", target.String(), "url", ppInfo.URL)
		imgData, errDownload := utils.FetchURLBytes(ctx.Ctx, ppInfo.URL)
		if errDownload != nil || len(imgData) == 0 {
			slog.Warn("fetchAvatarPath: Downloading avatar bytes failed", "err", errDownload, "len", len(imgData))
			continue
		}

		tmpFile, errTmp := os.CreateTemp("", "quote_avatar_*.jpg")
		if errTmp != nil {
			slog.Error("fetchAvatarPath: CreateTemp failed", "err", errTmp)
			continue
		}
		_, _ = tmpFile.Write(imgData)
		_ = tmpFile.Close()

		slog.Info("fetchAvatarPath: Avatar downloaded and saved to disk", "path", tmpFile.Name())
		return tmpFile.Name()
	}

	return ""
}
