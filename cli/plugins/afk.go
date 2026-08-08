// AFK command – customizable away-from-keyboard status, last active timestamp, and auto-response placeholders.
package plugins

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"

	"whatsrook/wa-core"
	"whatsrook/wa-core/proto/waE2E"
	"whatsrook/wa-core/store/sqlstore"
	"whatsrook/wa-core/types"
	"whatsrook/wa-core/types/events"
)

const (
	AFKStatusKey   = "afk_status"
	AFKReasonKey   = "afk_reason"
	AFKTimeKey     = "afk_time"
	AFKTemplateKey = "afk_template"
	AFKMediaKey    = "afk_media"

	AFKLastActiveKey = "owner_last_active"
)

var (
	afkMu              sync.RWMutex
	lastActiveCache    time.Time
	defaultAFKTemplate = "I am currently AFK.\n\nReason: {reason}\nTime: {time}\nLast Seen: {last_available}\n\n{quote}"

	factsList = []string{
		"Honey never spoils; archaeologists have found 3,000-year-old edible honey in Egyptian tombs.",
		"Octopuses have three hearts and blue blood.",
		"Bananas are naturally slightly radioactive because they are rich in potassium.",
		"Venus is the only planet in our solar system that rotates clockwise.",
		"A day on Venus is longer than a year on Venus.",
		"Wombat poop is cube-shaped to keep it from rolling away.",
		"Sharks existed before trees.",
	}

	quotesList = []string{
		"\"The secret of getting ahead is getting started.\" – Mark Twain",
		"\"It always seems impossible until it's done.\" – Nelson Mandela",
		"\"Do what you can, with what you have, where you are.\" – Theodore Roosevelt",
		"\"In the middle of every difficulty lies opportunity.\" – Albert Einstein",
		"\"Success is not final, failure is not fatal: It is the courage to continue that counts.\" – Winston Churchill",
	}

	jokesList = []string{
		"Why don't scientists trust atoms? Because they make up everything!",
		"Why did the scarecrow win an award? Because he was outstanding in his field!",
		"What do you call fake spaghetti? An impasta!",
		"Why do programmers prefer dark mode? Because light attracts bugs!",
		"How do you organize a space party? You planet!",
	}

	rizzList = []string{
		"Are you a magician? Because whenever I look at you, everyone else disappears.",
		"Do you have a map? I keep getting lost in your eyes.",
		"Is your name Google? Because you have everything I’ve been searching for.",
		"Are you Wi-Fi? Because I'm feeling a really strong connection.",
		"If beauty were time, you’d be an eternity.",
	}
)

func init() {
	Register(&Command{
		Name:        "afk",
		Aliases:     []string{"away"},
		Description: "Set or customize your Away-From-Keyboard (AFK) status with customizable templates, @ placeholders, and last active tracking",
		Category:    "settings",
		IsPublic:    false,
		Handler:     handleAFK,
	})
}

// UpdateOwnerLastActive records the timestamp whenever the bot owner sends a message or performs an action.
func UpdateOwnerLastActive(ctx context.Context, s *sqlstore.SQLStore) {
	now := time.Now()
	afkMu.Lock()
	lastActiveCache = now
	afkMu.Unlock()

	if s != nil {
		_ = s.PutSetting(ctx, AFKLastActiveKey, now.Format(time.RFC3339))
	}
}

// GetOwnerLastActive retrieves the owner's last active time before going AFK.
func GetOwnerLastActive(ctx context.Context, s *sqlstore.SQLStore) time.Time {
	afkMu.RLock()
	cached := lastActiveCache
	afkMu.RUnlock()
	if !cached.IsZero() {
		return cached
	}

	if s != nil {
		if val, err := s.GetSetting(ctx, AFKLastActiveKey); err == nil && val != "" {
			if t, errParse := time.Parse(time.RFC3339, val); errParse == nil {
				return t
			}
		}
	}
	return time.Now()
}

func handleAFK(ctx *Context) error {
	s, ok := ctx.Client.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return ctx.Reply("Database store unavailable.")
	}

	args := strings.Fields(ctx.RawArgs)
	if len(args) == 0 {
		// Set AFK with default or no reason if called directly by owner/sudo
		if !ctx.IsSudo() {
			return ctx.Reply("Only sudoers/owners can set AFK status.")
		}
		return setAFKStatus(ctx, s, "AFK (No reason specified)")
	}

	sub := strings.ToLower(args[0])
	switch sub {
	case "off", "disable", "back", "done":
		if !ctx.IsSudo() {
			return ctx.Reply("Only sudoers/owners can turn off AFK status.")
		}
		_ = s.PutSetting(ctx.Ctx, AFKStatusKey, "off")
		_ = s.PutSetting(ctx.Ctx, AFKReasonKey, "")
		_ = s.PutSetting(ctx.Ctx, AFKTimeKey, "")
		UpdateOwnerLastActive(ctx.Ctx, s)
		return ctx.Reply("Welcome back! AFK mode has been turned *off*.")

	case "customize", "custom", "help":
		return sendAFKCustomizeGuide(ctx)

	case "msg", "template", "text":
		if !ctx.IsSudo() {
			return ctx.Reply("Only sudoers/owners can customize the AFK template.")
		}
		if len(args) < 2 {
			curr, _ := s.GetSetting(ctx.Ctx, AFKTemplateKey)
			if curr == "" {
				curr = defaultAFKTemplate
			}
			return ctx.Reply("Current AFK Message Template:\n\n" + curr)
		}
		newTpl := strings.TrimSpace(ctx.RawArgs[len(args[0]):])
		if strings.EqualFold(newTpl, "reset") || strings.EqualFold(newTpl, "clear") {
			_ = s.PutSetting(ctx.Ctx, AFKTemplateKey, "")
			return ctx.Reply("AFK message template reset to default.")
		}
		if err := s.PutSetting(ctx.Ctx, AFKTemplateKey, newTpl); err != nil {
			return ctx.Reply("Failed to save AFK template: " + err.Error())
		}
		return ctx.Reply("Custom AFK message template updated successfully!\n\nUse `" + ctx.GetPrefix() + "afk msg reset` to restore default.")

	case "media":
		if !ctx.IsSudo() {
			return ctx.Reply("Only sudoers/owners can set AFK media.")
		}
		if len(args) < 2 {
			curr, _ := s.GetSetting(ctx.Ctx, AFKMediaKey)
			if curr == "" {
				return ctx.Reply("No custom AFK media URL set.")
			}
			return ctx.Reply("Current AFK Media URL: " + curr)
		}
		urlVal := strings.TrimSpace(args[1])
		if strings.EqualFold(urlVal, "clear") || strings.EqualFold(urlVal, "off") || strings.EqualFold(urlVal, "none") {
			_ = s.PutSetting(ctx.Ctx, AFKMediaKey, "")
			return ctx.Reply("AFK media URL cleared.")
		}
		if err := s.PutSetting(ctx.Ctx, AFKMediaKey, urlVal); err != nil {
			return ctx.Reply("Failed to save AFK media URL: " + err.Error())
		}
		return ctx.Reply("AFK media URL updated successfully!")

	default:
		if !ctx.IsSudo() {
			return ctx.Reply("Only sudoers/owners can set AFK status.")
		}
		reason := strings.TrimSpace(ctx.RawArgs)
		return setAFKStatus(ctx, s, reason)
	}
}

func setAFKStatus(ctx *Context, s *sqlstore.SQLStore, reason string) error {
	lastActive := GetOwnerLastActive(ctx.Ctx, s)
	nowStr := time.Now().Format("2006-01-02 15:04:05 MST")
	lastActiveStr := lastActive.Format("2006-01-02 15:04:05 MST")

	_ = s.PutSetting(ctx.Ctx, AFKStatusKey, "on")
	_ = s.PutSetting(ctx.Ctx, AFKReasonKey, reason)
	_ = s.PutSetting(ctx.Ctx, AFKTimeKey, nowStr)
	_ = s.PutSetting(ctx.Ctx, AFKLastActiveKey, lastActiveStr)

	p := ctx.GetPrefix()
	return ctx.Reply(fmt.Sprintf("AFK mode activated! 🌙\n\n📌 *Reason*: %s\n⏰ *Time*: %s\n⌛ *Last Available*: %s\n\nTurn off anytime using `%safk back` or `%safk off`.", reason, nowStr, lastActiveStr, p, p))
}

func sendAFKCustomizeGuide(ctx *Context) error {
	p := ctx.GetPrefix()
	var sb strings.Builder
	sb.WriteString("╭━━━〔 AFK CUSTOMIZATION GUIDE 〕━━━\n\n")
	sb.WriteString("Usage:\n")
	fmt.Fprintf(&sb, "• Activate AFK       : `%safk <reason>`\n", p)
	fmt.Fprintf(&sb, "• Deactivate AFK     : `%safk off` or `%safk back`\n", p, p)
	fmt.Fprintf(&sb, "• Custom Message     : `%safk msg <your custom template>`\n", p)
	fmt.Fprintf(&sb, "• Custom Media URL   : `%safk media <url | clear>`\n", p)
	fmt.Fprintf(&sb, "• Reset Template     : `%safk msg reset`\n\n", p)

	sb.WriteString("Available Placeholders & Tags:\n")
	sb.WriteString("- `{reason}` or `@reason`         : Reason for being AFK\n")
	sb.WriteString("- `{time}` or `@time`             : Time AFK mode was set\n")
	sb.WriteString("- `{last_available}` or `@time`   : Owner's last available active timestamp\n")
	sb.WriteString("- `{fact}` or `@fact`             : Random interesting fact\n")
	sb.WriteString("- `{quote}` or `@quote`           : Random inspirational quote\n")
	sb.WriteString("- `{joke}` or `@joke`             : Random funny joke\n")
	sb.WriteString("- `{rizz}` or `@rizz`             : Random smooth pickup line / rizz\n")
	sb.WriteString("- `{user}` or `@user`             : Mention sender tag (@username)\n")
	sb.WriteString("- `{group}` or `@group`           : Group name (if in group)\n\n")

	sb.WriteString("Example Custom Template:\n")
	fmt.Fprintf(&sb, "`%safk msg Hello {user}! Owner has been AFK since {time} (Last active: {last_available}). Reason: {reason}. Here is a joke for you: {joke}`\n", p)

	return ctx.Reply(strings.TrimSpace(sb.String()))
}

// HandleAFKAutoResponse checks if owner is currently AFK and someone tagged/messaged them, sending customizable response.
func HandleAFKAutoResponse(ctx context.Context, client *whatsmeow.Client, evt *events.Message, text string) bool {
	if evt == nil || evt.Message == nil || client == nil || client.Store == nil || client.Store.ID == nil {
		return false
	}

	s, ok := client.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return false
	}

	ownerJID := client.Store.ID.ToNonAD()
	senderJID := evt.Info.Sender.ToNonAD()

	// If owner sends a message, update last active and automatically turn off AFK if active!
	if senderJID.User == ownerJID.User {
		UpdateOwnerLastActive(ctx, s)
		status, _ := s.GetSetting(ctx, AFKStatusKey)
		if status == "on" {
			// Don't auto-turn off if owner is explicitly running the afk command
			if !strings.HasPrefix(strings.TrimSpace(text), ".") && !strings.HasPrefix(strings.TrimSpace(text), "/") && !strings.HasPrefix(strings.TrimSpace(text), "!") && !strings.HasPrefix(strings.TrimSpace(text), "#") {
				_ = s.PutSetting(ctx, AFKStatusKey, "off")
				cctx := &Context{
					Ctx:    ctx,
					Client: client,
					Evt:    evt,
					Chat:   evt.Info.Chat,
					Sender: evt.Info.Sender,
				}
				_ = cctx.Reply("Welcome back! You sent a message, so AFK mode has been automatically turned *off*.")
			}
		}
		return false
	}

	// Check if owner is AFK
	status, _ := s.GetSetting(ctx, AFKStatusKey)
	if status != "on" {
		return false
	}

	// Check if message is in DM to owner OR if owner is mentioned / replied to in a group
	isDM := evt.Info.Chat.Server != "g.us"
	isMentioned := false
	if !isDM {
		if strings.Contains(text, "@"+ownerJID.User) {
			isMentioned = true
		}
		if ci := getContextInfoFromProto(evt.Message); ci != nil {
			for _, m := range ci.GetMentionedJID() {
				if parsed, err := types.ParseJID(m); err == nil && parsed.ToNonAD().User == ownerJID.User {
					isMentioned = true
					break
				}
			}
			if ci.Participant != nil {
				if parsed, err := types.ParseJID(ci.GetParticipant()); err == nil && parsed.ToNonAD().User == ownerJID.User {
					isMentioned = true
				}
			}
		}
	}

	if !isDM && !isMentioned {
		return false
	}

	// Retrieve AFK details
	reason, _ := s.GetSetting(ctx, AFKReasonKey)
	if reason == "" {
		reason = "AFK (No reason specified)"
	}
	afkTime, _ := s.GetSetting(ctx, AFKTimeKey)
	if afkTime == "" {
		afkTime = time.Now().Format("2006-01-02 15:04:05 MST")
	}
	lastActiveStr, _ := s.GetSetting(ctx, AFKLastActiveKey)
	if lastActiveStr == "" {
		lastActiveStr = GetOwnerLastActive(ctx, s).Format("2006-01-02 15:04:05 MST")
	}

	template, _ := s.GetSetting(ctx, AFKTemplateKey)
	if template == "" {
		template = defaultAFKTemplate
	}

	userTag := "@" + senderJID.User
	groupName := evt.Info.Chat.String()
	if info, err := client.GetGroupInfo(ctx, evt.Info.Chat); err == nil && info != nil && info.GroupName.Name != "" {
		groupName = info.GroupName.Name
	}

	rand.Seed(time.Now().UnixNano())
	randomFact := factsList[rand.Intn(len(factsList))]
	randomQuote := quotesList[rand.Intn(len(quotesList))]
	randomJoke := jokesList[rand.Intn(len(jokesList))]
	randomRizz := rizzList[rand.Intn(len(rizzList))]

	replacer := strings.NewReplacer(
		"{reason}", reason,
		"@reason", reason,
		"{time}", afkTime,
		"{last_available}", lastActiveStr,
		"@time", lastActiveStr,
		"{fact}", randomFact,
		"@fact", randomFact,
		"{quote}", randomQuote,
		"@quote", randomQuote,
		"{joke}", randomJoke,
		"@joke", randomJoke,
		"{rizz}", randomRizz,
		"@rizz", randomRizz,
		"{user}", userTag,
		"@user", userTag,
		"{group}", groupName,
		"@group", groupName,
	)

	body := replacer.Replace(template)

	cctx := &Context{
		Ctx:    ctx,
		Client: client,
		Evt:    evt,
		Chat:   evt.Info.Chat,
		Sender: evt.Info.Sender,
	}

	mediaURL, _ := s.GetSetting(ctx, AFKMediaKey)
	if mediaURL != "" {
		body = body + "\n\n" + mediaURL
	}
	_ = cctx.Reply(body)

	return true
}

func getContextInfoFromProto(msg *waE2E.Message) *waE2E.ContextInfo {
	if msg == nil {
		return nil
	}
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
	if btn := msg.GetButtonsResponseMessage(); btn != nil {
		return btn.GetContextInfo()
	}
	if inter := msg.GetInteractiveResponseMessage(); inter != nil {
		return inter.GetContextInfo()
	}
	if lst := msg.GetListResponseMessage(); lst != nil {
		return lst.GetContextInfo()
	}
	if poll := msg.GetPollCreationMessage(); poll != nil {
		return poll.GetContextInfo()
	}
	if evt := msg.GetEventMessage(); evt != nil {
		return evt.GetContextInfo()
	}
	return nil
}
