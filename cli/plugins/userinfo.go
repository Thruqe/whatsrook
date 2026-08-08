// UserInfo command – fetches and displays detailed profile info for a WhatsApp user.
package plugins

import (
	"fmt"
	"log/slog"
	"strings"

	"whatsrook/utils"

	"whatsrook/wa-core"
	"whatsrook/wa-core/types"
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

	targets := ctx.GetTargets()
	var rawTarget types.JID
	if len(targets) > 0 {
		rawTarget = targets[0]
	} else {
		rawTarget = ctx.Sender
	}

	// Resolve target JID from LID to Phone Number JID and clean it up (ToNonAD)
	targetJID := NormalizeUserJID(ctx.Ctx, ctx.Client, rawTarget)

	loader := ctx.StartLoader("Fetching user profile...")
	defer loader.Delete()

	// Fetch status bio & device list to double check PN resolution if still LID
	bioText := ""
	queryJIDs := []types.JID{targetJID}
	if rawTarget != targetJID {
		queryJIDs = append(queryJIDs, rawTarget)
	}

	if uMap, err := ctx.Client.GetUserInfo(ctx.Ctx, queryJIDs); err == nil && uMap != nil {
		for _, qJID := range queryJIDs {
			if uInfo, ok := uMap[qJID]; ok {
				if uInfo.Status != "" && bioText == "" {
					bioText = strings.TrimSpace(uInfo.Status)
				}
				// If targetJID is still LID, check devices for phone number JID
				if targetJID.Server == types.HiddenUserServer && len(uInfo.Devices) > 0 {
					for _, dev := range uInfo.Devices {
						if dev.Server == types.DefaultUserServer && dev.User != "" {
							pnJID := types.NewJID(dev.User, types.DefaultUserServer)
							targetJID = pnJID
							if ctx.Client != nil && ctx.Client.Store != nil && ctx.Client.Store.LIDs != nil {
								_ = ctx.Client.Store.LIDs.PutLIDMapping(ctx.Ctx, rawTarget.ToNonAD(), pnJID)
							}
							break
						}
					}
				}
			}
		}
	}

	// Get contact info
	pushName := targetJID.User
	if contact, err := ctx.Client.Store.Contacts.GetContact(ctx.Ctx, targetJID); err == nil && contact.Found {
		if contact.PushName != "" {
			pushName = contact.PushName
		} else if contact.FullName != "" {
			pushName = contact.FullName
		} else if contact.BusinessName != "" {
			pushName = contact.BusinessName
		}
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "╭━━━〔 USER INFO 〕━━━\n")
	fmt.Fprintf(&sb, "│ Name  : %s\n", pushName)
	if targetJID.Server == types.DefaultUserServer {
		fmt.Fprintf(&sb, "│ Phone : +%s\n", targetJID.User)
	} else {
		fmt.Fprintf(&sb, "│ LID   : %s\n", targetJID.User)
	}
	if bioText != "" && bioText != "N/A" {
		fmt.Fprintf(&sb, "│ Status: %s\n", bioText)
	}
	fmt.Fprintf(&sb, "╰━━━━━━━━━━━━━━━━━━━━\n\n")
	fmt.Fprintf(&sb, "Tip: Use %suserinfo @user to check another contact.", p)

	infoText := sb.String()

	// Fetch profile picture
	ppInfo, errPP := ctx.Client.GetProfilePictureInfo(ctx.Ctx, targetJID, &whatsmeow.GetProfilePictureParams{})
	if (errPP != nil || ppInfo == nil || ppInfo.URL == "") && rawTarget != targetJID {
		ppInfo, errPP = ctx.Client.GetProfilePictureInfo(ctx.Ctx, rawTarget, &whatsmeow.GetProfilePictureParams{})
	}

	if errPP == nil && ppInfo != nil && ppInfo.URL != "" {
		slog.Info("handleUserInfo: Downloading profile photo", "url", ppInfo.URL)
		imgData, errDownload := utils.FetchURLBytes(ctx.Ctx, ppInfo.URL)
		if errDownload == nil && len(imgData) > 0 {
			return ctx.ReplyWithImage(imgData, "image/jpeg", infoText)
		}
	}

	return ctx.Reply(infoText)
}
