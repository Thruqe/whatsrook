// Status broadcast command – posts status updates (text, image, or video with optional caption) to status.whatsapp.net broadcast.
package plugins

import (
	"fmt"
	"log/slog"
	"strings"

	"whatsrook/wa-core"
	"whatsrook/wa-core/proto/waE2E"
	"whatsrook/wa-core/types"
)

var statusBroadcastJID = types.JID{User: "status", Server: "broadcast"}

func init() {
	Register(&Command{
		Name:        "status",
		Aliases:     []string{"poststatus", "sw"},
		Description: "Post a status update (text or media) to WhatsApp status broadcast",
		Category:    "owner",
		IsPublic:    false,
		Handler:     handleStatus,
	})
}

func handleStatus(ctx *Context) error {
	if !ctx.IsSudo() {
		return ctx.Reply("Only owner/sudo users can post status updates.")
	}

	loader := ctx.StartLoader("Posting status update")
	defer loader.Delete()

	text := strings.TrimSpace(ctx.RawArgs)

	// Check if message has media (either directly or via quoted message)
	mediaBytes, mimetype, err := ctx.GetMedia()
	if err == nil && len(mediaBytes) > 0 {
		// Media present (image or video)
		isImage := strings.HasPrefix(mimetype, "image")
		isVideo := strings.HasPrefix(mimetype, "video") || strings.Contains(mimetype, "gif")

		if !isImage && !isVideo {
			// If not strictly image or video, check mimetype or default based on media type
			if strings.HasPrefix(mimetype, "audio") {
				return ctx.Reply("Only image and video media can be posted to status broadcast.")
			}
			isImage = true // fallback to image upload
		}

		if isImage {
			if mimetype == "" {
				mimetype = "image/jpeg"
			}
			uploaded, uErr := ctx.Client.Upload(ctx.Ctx, mediaBytes, whatsmeow.MediaImage)
			if uErr != nil {
				slog.Error("handleStatus: image upload failed", "err", uErr)
				return ctx.Reply(fmt.Sprintf("Failed to upload status image: %v", uErr))
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
				},
			}
			*msg.ImageMessage.FileLength = uint64(len(mediaBytes))
			if text != "" {
				msg.ImageMessage.Caption = &text
			}

			_, sendErr := ctx.Client.SendMessage(ctx.Ctx, statusBroadcastJID, msg)
			if sendErr != nil {
				slog.Error("handleStatus: send image status failed", "err", sendErr)
				return ctx.Reply(fmt.Sprintf("Failed to post image status: %v", sendErr))
			}
			return ctx.Reply("Successfully posted image status update.")
		}

		if isVideo {
			if mimetype == "" {
				mimetype = "video/mp4"
			}
			uploaded, uErr := ctx.Client.Upload(ctx.Ctx, mediaBytes, whatsmeow.MediaVideo)
			if uErr != nil {
				slog.Error("handleStatus: video upload failed", "err", uErr)
				return ctx.Reply(fmt.Sprintf("Failed to upload status video: %v", uErr))
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
				},
			}
			*msg.VideoMessage.FileLength = uint64(len(mediaBytes))
			if text != "" {
				msg.VideoMessage.Caption = &text
			}

			_, sendErr := ctx.Client.SendMessage(ctx.Ctx, statusBroadcastJID, msg)
			if sendErr != nil {
				slog.Error("handleStatus: send video status failed", "err", sendErr)
				return ctx.Reply(fmt.Sprintf("Failed to post video status: %v", sendErr))
			}
			return ctx.Reply("Successfully posted video status update.")
		}
	}

	// Text status update
	if text == "" {
		p := ctx.GetPrefix()
		return ctx.Reply(fmt.Sprintf("Usage:\n- %sstatus <text>\n- Reply to image/video with %sstatus [optional caption]", p, p))
	}

	msg := &waE2E.Message{
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text: &text,
		},
	}

	_, sendErr := ctx.Client.SendMessage(ctx.Ctx, statusBroadcastJID, msg)
	if sendErr != nil {
		slog.Error("handleStatus: send text status failed", "err", sendErr)
		return ctx.Reply(fmt.Sprintf("Failed to post text status: %v", sendErr))
	}
	return ctx.Reply("Successfully posted text status update.")
}
