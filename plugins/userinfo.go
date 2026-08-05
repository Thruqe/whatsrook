// UserInfo command – fetches and displays detailed profile info for a WhatsApp user.
package plugins

import (
	"fmt"
	"log/slog"
	"strings"

	"whatsrook/utils"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

func init() {
	Register(&Command{
		Name:        "userinfo",
		Aliases:     []string{"whois", "uinfo", "user"},
		Description: "Display detailed profile information and profile photo of a WhatsApp user",
		Category:    "tools",
		IsPublic:    true,
		Handler:     handleUserInfo,
	})
}

func handleUserInfo(ctx *Context) error {
	p := ctx.GetPrefix()
	targetJID := ctx.Sender

	// Check if target is mentioned or replied or provided as arg
	if quoted := getQuotedMessageFromEvent(ctx.Evt); quoted != nil {
		if ext := ctx.Evt.Message.GetExtendedTextMessage(); ext != nil && ext.GetContextInfo() != nil && ext.GetContextInfo().Participant != nil {
			if parsed, err := types.ParseJID(*ext.GetContextInfo().Participant); err == nil && !parsed.IsEmpty() {
				targetJID = parsed
			}
		}
	} else if len(ctx.Args) > 0 {
		rawArg := strings.TrimPrefix(ctx.Args[0], "@")
		rawArg = strings.TrimSpace(rawArg)
		if parsed, err := types.ParseJID(rawArg); err == nil && !parsed.IsEmpty() {
			targetJID = parsed
		} else if !strings.Contains(rawArg, "@") {
			targetJID = types.NewJID(rawArg, types.DefaultUserServer)
		}
	}

	targetJID = targetJID.ToNonAD()

	loader := ctx.StartLoader("Fetching user profile...")
	defer loader.Delete()

	// Get contact info
	pushName := targetJID.User
	if contact, err := ctx.Client.Store.Contacts.GetContact(ctx.Ctx, targetJID); err == nil && contact.Found {
		if contact.PushName != "" {
			pushName = contact.PushName
		} else if contact.FullName != "" {
			pushName = contact.FullName
		}
	}

	// Fetch status bio
	bioText := ""
	if uMap, err := ctx.Client.GetUserInfo(ctx.Ctx, []types.JID{targetJID}); err == nil && uMap != nil {
		if uInfo, ok := uMap[targetJID]; ok && uInfo.Status != "" {
			bioText = strings.TrimSpace(uInfo.Status)
		}
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "╭━━━〔 USER INFO 〕━━━\n")
	fmt.Fprintf(&sb, "│ Name  : %s\n", pushName)
	fmt.Fprintf(&sb, "│ Phone : +%s\n", targetJID.User)
	if bioText != "" && bioText != "N/A" {
		fmt.Fprintf(&sb, "│ Status: %s\n", bioText)
	}
	fmt.Fprintf(&sb, "╰━━━━━━━━━━━━━━━━━━━━\n\n")
	fmt.Fprintf(&sb, "Tip: Use %suserinfo @user to check another contact.", p)

	infoText := sb.String()

	// Fetch profile picture
	ppInfo, errPP := ctx.Client.GetProfilePictureInfo(ctx.Ctx, targetJID, &whatsmeow.GetProfilePictureParams{})
	if errPP == nil && ppInfo != nil && ppInfo.URL != "" {
		slog.Info("handleUserInfo: Downloading profile photo", "url", ppInfo.URL)
		imgData, errDownload := utils.FetchURLBytes(ctx.Ctx, ppInfo.URL)
		if errDownload == nil && len(imgData) > 0 {
			return ctx.ReplyWithImage(imgData, "image/jpeg", infoText)
		}
	}

	return ctx.Reply(infoText)
}
