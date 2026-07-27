// AI Bot tools – hidden helper commands that allow Meta AI and raw callers
// to execute send, edit, ffmpeg, fetch, and delete actions with raw parameters.
package commands

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"go.mau.fi/whatsmeow/types"
)

func init() {
	Register(&Command{
		Name:         "send",
		Description:  "Send raw text message to the current chat",
		Category:     "whatsrook_ai_bot_tools",
		HideFromMenu: true,
		IsPublic:     true,
		Handler:      handleSend,
	})
	Register(&Command{
		Name:         "edit",
		Description:  "Edit a message by message ID or replied message",
		Category:     "whatsrook_ai_bot_tools",
		HideFromMenu: true,
		IsPublic:     true,
		Handler:      handleEditMsg,
	})
	Register(&Command{
		Name:         "ffmpeg",
		Description:  "Run raw ffmpeg media command",
		Category:     "whatsrook_ai_bot_tools",
		HideFromMenu: true,
		IsPublic:     false,
		Handler:      handleFFmpeg,
	})
	Register(&Command{
		Name:         "downloadMessage",
		Aliases:      []string{"download", "dl"},
		Description:  "Download media from a message by ID or quoted message",
		Category:     "whatsrook_ai_bot_tools",
		HideFromMenu: true,
		IsPublic:     true,
		Handler:      handleDownloadMessage,
	})
}

func handleSend(ctx *Context) error {
	if ctx.RawArgs == "" {
		return ctx.Reply("Usage: send <text>")
	}
	return ctx.SendText(ctx.RawArgs)
}

func handleEditMsg(ctx *Context) error {
	if len(ctx.Args) == 0 {
		return ctx.Reply("Usage: edit <msg_id> <new_text> or reply to a message with edit <new_text>")
	}

	var targetID types.MessageID
	var newText string

	ci := ctx.GetContextInfo()
	quotedSender, hasQuoted := ctx.GetQuotedSender()

	if len(ctx.Args) >= 2 && len(ctx.Args[0]) > 6 {
		targetID = types.MessageID(ctx.Args[0])
		newText = strings.TrimSpace(ctx.RawArgs[len(ctx.Args[0]):])
	} else if ci != nil && ci.StanzaID != nil {
		targetID = *ci.StanzaID
		newText = ctx.RawArgs
	} else {
		targetID = types.MessageID(ctx.Args[0])
		newText = strings.TrimSpace(ctx.RawArgs[len(ctx.Args[0]):])
	}

	if hasQuoted {
		if ctx.Client.Store.ID != nil && !ctx.IsSameUser(quotedSender, *ctx.Client.Store.ID) {
			return ctx.Reply("You can only edit messages sent by the bot (fromMe=true).")
		}
	}

	if newText == "" {
		return ctx.Reply("Please provide the new text for the message.")
	}

	_, err := ctx.Edit(targetID, newText)
	if err != nil {
		return ctx.Reply("Failed to edit message: " + err.Error())
	}
	return nil
}

func handleFFmpeg(ctx *Context) error {
	if !ctx.IsSudo() {
		return ctx.Reply("Restricted to sudoers/owner only.")
	}
	if ctx.RawArgs == "" {
		return ctx.Reply("Usage: ffmpeg <args...>")
	}

	// Check if replying to media
	mediaData, mimetype, err := ctx.GetMedia()
	if err == nil && len(mediaData) > 0 {
		ext := ".tmp"
		if strings.Contains(mimetype, "image/") {
			ext = "." + strings.TrimPrefix(mimetype, "image/")
		} else if strings.Contains(mimetype, "video/") {
			ext = "." + strings.TrimPrefix(mimetype, "video/")
		} else if strings.Contains(mimetype, "audio/") {
			ext = "." + strings.TrimPrefix(mimetype, "audio/")
		}

		tmpFile, err := os.CreateTemp("", "ffmpeg_input_*"+ext)
		if err != nil {
			return ctx.Reply("Failed to create temporary file: " + err.Error())
		}
		defer os.Remove(tmpFile.Name())
		_, _ = tmpFile.Write(mediaData)
		_ = tmpFile.Close()

		outFile := tmpFile.Name() + ".out.mp4"
		if strings.Contains(ctx.RawArgs, ".mp3") {
			outFile = tmpFile.Name() + ".out.mp3"
		} else if strings.Contains(ctx.RawArgs, ".webp") {
			outFile = tmpFile.Name() + ".out.webp"
		} else if strings.Contains(ctx.RawArgs, ".png") {
			outFile = tmpFile.Name() + ".out.png"
		} else if strings.Contains(ctx.RawArgs, ".jpg") || strings.Contains(ctx.RawArgs, ".jpeg") {
			outFile = tmpFile.Name() + ".out.jpg"
		}
		defer os.Remove(outFile)

		rawCmd := ctx.RawArgs
		if strings.Contains(rawCmd, "{input}") {
			rawCmd = strings.ReplaceAll(rawCmd, "{input}", tmpFile.Name())
		} else if strings.Contains(rawCmd, "{in}") {
			rawCmd = strings.ReplaceAll(rawCmd, "{in}", tmpFile.Name())
		}
		if strings.Contains(rawCmd, "{output}") {
			rawCmd = strings.ReplaceAll(rawCmd, "{output}", outFile)
		} else if strings.Contains(rawCmd, "{out}") {
			rawCmd = strings.ReplaceAll(rawCmd, "{out}", outFile)
		}

		parts := strings.Fields(rawCmd)
		cmd := exec.Command("ffmpeg", parts...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return ctx.Reply(fmt.Sprintf("FFmpeg execution error: %v\nOutput: %s", err, string(out)))
		}

		if outBytes, err := os.ReadFile(outFile); err == nil && len(outBytes) > 0 {
			if strings.HasSuffix(outFile, ".mp3") || strings.HasSuffix(outFile, ".ogg") {
				return ctx.SendAudio(outBytes, "audio/mp4")
			} else if strings.HasSuffix(outFile, ".webp") {
				return ctx.SendSticker(outBytes)
			} else if strings.HasSuffix(outFile, ".png") || strings.HasSuffix(outFile, ".jpg") {
				return ctx.SendImage(outBytes, "image/jpeg", "FFmpeg processed image")
			} else {
				return ctx.SendVideo(outBytes, "video/mp4", "FFmpeg processed video")
			}
		}

		resStr := string(out)
		if len(resStr) > 1500 {
			resStr = resStr[:1500] + "\n... (truncated)"
		}
		if resStr == "" {
			resStr = "FFmpeg command completed successfully."
		}
		return ctx.Reply(resStr)
	}

	parts := strings.Fields(ctx.RawArgs)
	cmd := exec.Command("ffmpeg", parts...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return ctx.Reply(fmt.Sprintf("FFmpeg execution error: %v\nOutput: %s", err, string(out)))
	}
	resStr := string(out)
	if len(resStr) > 1500 {
		resStr = resStr[:1500] + "\n... (truncated)"
	}
	if resStr == "" {
		resStr = "FFmpeg command completed successfully."
	}
	return ctx.Reply(resStr)
}

func handleDownloadMessage(ctx *Context) error {
	mediaData, mimetype, err := ctx.GetMedia()
	if err != nil || len(mediaData) == 0 {
		return ctx.Reply("No downloadable media found in the target or quoted message.")
	}

	switch {
	case strings.HasPrefix(mimetype, "image/webp"):
		return ctx.SendSticker(mediaData)
	case strings.HasPrefix(mimetype, "image/"):
		return ctx.SendImage(mediaData, mimetype, "Downloaded image")
	case strings.HasPrefix(mimetype, "video/"):
		return ctx.SendVideo(mediaData, mimetype, "Downloaded video")
	case strings.HasPrefix(mimetype, "audio/"):
		return ctx.SendAudio(mediaData, mimetype)
	default:
		return ctx.SendDocument(mediaData, mimetype, "downloaded_media", "")
	}
}
