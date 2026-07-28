// Word Guessing Game (WCG) – Multiplayer round-robin anagram game with dynamic difficulty, rating system, and XP.
package commands

import (
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
	"unicode"

	"whatsrook/store/sqlstore"

	"go.mau.fi/whatsmeow"
	waBinary "go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
)

type wcgState int

const (
	wcgStateLobby wcgState = iota
	wcgStateInProgress
)

type wcgPlayer struct {
	LID            types.JID
	MentionJID     types.JID
	Tag            string
	Score          int
	Eliminated     bool
	CorrectGuesses int
	TotalTimeMs    int64
	GuessesCount   int
	JoinedAt       time.Time
}

type wcgGame struct {
	mu             sync.Mutex
	chatKey        string
	state          wcgState
	hostLID        types.JID
	hostMention    types.JID
	hostTag        string
	players        []*wcgPlayer
	currentTurnIdx int
	wordLength     int // 3 to 16
	currentWord    string
	scrambledWord  string
	turnStartTime  time.Time
	lobbyTimer     *time.Timer
	turnTimer      *time.Timer
	gameStartTime  time.Time
	client         *whatsmeow.Client
	chatJID        types.JID
}

var (
	wcgMu    sync.Mutex
	wcgGames = make(map[string]*wcgGame) // chat key -> game
)

func init() {
	Register(&Command{
		Name:        "wcg",
		Description: "Word Guessing Game with 30s lobby, dynamic time limits, performance ratings & XP",
		Category:    "games",
		IsPublic:    true,
		Handler:     handleWCG,
	})
}

// IsWCGGameActive returns true if there is an active WCG game (lobby or in-progress) in the chat.
func IsWCGGameActive(chatKey string) bool {
	wcgMu.Lock()
	defer wcgMu.Unlock()
	_, exists := wcgGames[chatKey]
	return exists
}

// HandleWCGInput intercepts non-prefix text messages in a chat where a WCG game is active.
// Returns true if the message was handled/swallowed by WCG.
func HandleWCGInput(ctx *Context, text string) bool {
	chatKey := ctx.Chat.String()

	wcgMu.Lock()
	game, exists := wcgGames[chatKey]
	wcgMu.Unlock()

	if !exists {
		return false
	}

	game.mu.Lock()
	defer game.mu.Unlock()

	// In lobby phase, ignore normal chat messages (only .wcg commands processed)
	if game.state == wcgStateLobby {
		return false
	}

	senderLID := ctx.Sender.ToNonAD()

	// Check if message is pure emoji or empty -> ignore without reply
	if isPureEmoji(text) || strings.TrimSpace(text) == "" {
		slog.Debug("[WCG] Ignored emoji/empty input", "chat", chatKey, "sender", senderLID.String())
		return true
	}

	// 1. Check if sender is in the game
	pIdx := game.findPlayerIndex(senderLID)
	if pIdx == -1 {
		// User is NOT in the game -> silently ignore without replying
		slog.Debug("[WCG] Ignored input from non-player", "chat", chatKey, "sender", senderLID.String())
		return true
	}

	// 2. Check if it's the sender's turn
	activePlayers := game.getActivePlayers()
	if len(activePlayers) == 0 {
		return true
	}

	currentTurnPlayer := game.players[game.currentTurnIdx]
	if currentTurnPlayer.LID.User != senderLID.User {
		// In game, but NOT sender's turn -> silently ignore without replying
		slog.Debug("[WCG] Ignored input from player whose turn it is not", "chat", chatKey, "sender", senderLID.String())
		return true
	}

	// 3. It IS sender's turn to guess!
	guess := strings.ToLower(strings.TrimSpace(text))
	elapsed := time.Since(game.turnStartTime)

	if game.turnTimer != nil {
		game.turnTimer.Stop()
	}

	currentTurnPlayer.GuessesCount++
	currentTurnPlayer.TotalTimeMs += elapsed.Milliseconds()

	if guess == game.currentWord {
		// Correct guess!
		currentTurnPlayer.Score += game.wordLength * 10
		currentTurnPlayer.CorrectGuesses++

		slog.Debug("[WCG] Correct guess!", "player", currentTurnPlayer.Tag, "word", game.currentWord, "timeMs", elapsed.Milliseconds())

		msg := fmt.Sprintf("🎉 Correct! %s guessed '%s' in %.1fs! (+%d pts)\n\nAdvancing to the next level!",
			currentTurnPlayer.Tag, game.currentWord, elapsed.Seconds(), game.wordLength*10)
		_ = ctx.ReplyWithMentions(msg, []types.JID{currentTurnPlayer.MentionJID})

		// Increase word length up to 16
		if game.wordLength < 16 {
			game.wordLength++
		}

		// Move to next player and start turn
		game.advanceTurn(ctx)
		return true
	}

	// Wrong guess!
	slog.Debug("[WCG] Incorrect guess", "player", currentTurnPlayer.Tag, "guess", guess, "correct", game.currentWord)
	msg := fmt.Sprintf("❌ Incorrect guess by %s!\nThe correct word was: '%s'.\n%s has been eliminated from this match!",
		currentTurnPlayer.Tag, game.currentWord, currentTurnPlayer.Tag)
	_ = ctx.ReplyWithMentions(msg, []types.JID{currentTurnPlayer.MentionJID})

	currentTurnPlayer.Eliminated = true

	// Check remaining active players
	rem := game.getActivePlayers()
	if len(rem) <= 1 {
		game.finishGame(ctx)
		return true
	}

	game.advanceTurn(ctx)
	return true
}

func handleWCG(ctx *Context) error {
	chatKey := ctx.Chat.String()

	wcgMu.Lock()
	existingGame, exists := wcgGames[chatKey]
	wcgMu.Unlock()

	arg0 := ""
	if len(ctx.Args) > 0 {
		arg0 = strings.ToLower(ctx.Args[0])
	}

	if arg0 == "lb" || arg0 == "leaderboard" {
		return handleWCGLeaderboard(ctx)
	}

	if arg0 == "cancel" || arg0 == "stop" {
		if !exists {
			return ctx.Reply("No active WCG game to cancel.")
		}
		existingGame.mu.Lock()
		if existingGame.lobbyTimer != nil {
			existingGame.lobbyTimer.Stop()
		}
		if existingGame.turnTimer != nil {
			existingGame.turnTimer.Stop()
		}
		existingGame.mu.Unlock()

		wcgMu.Lock()
		delete(wcgGames, chatKey)
		wcgMu.Unlock()

		return ctx.Reply("Word Guessing Game cancelled.")
	}

	// Join sub-command
	if arg0 == "join" {
		if !exists {
			return ctx.Reply("No WCG game lobby open. Type .wcg to start one!")
		}
		existingGame.mu.Lock()
		defer existingGame.mu.Unlock()

		if existingGame.state != wcgStateLobby {
			return ctx.Reply("WCG game is already in progress!")
		}

		senderLID := ctx.Sender.ToNonAD()
		if existingGame.findPlayerIndex(senderLID) != -1 {
			return ctx.Reply("You are already in the WCG lobby!")
		}

		mentionJID, username := ctx.ResolveMention(senderLID)
		p := &wcgPlayer{
			LID:        senderLID,
			MentionJID: mentionJID,
			Tag:        "@" + username,
			JoinedAt:   time.Now(),
		}
		existingGame.players = append(existingGame.players, p)

		msg := fmt.Sprintf("✅ %s joined the WCG match! (%d players in lobby)\nType .wcg start to begin immediately or wait for timer.", p.Tag, len(existingGame.players))
		return ctx.ReplyWithMentions(msg, []types.JID{mentionJID})
	}

	// Start sub-command (starts lobby immediately if > 0 players)
	if arg0 == "start" || arg0 == "create" {
		if exists {
			existingGame.mu.Lock()
			if existingGame.state == wcgStateLobby {
				if len(existingGame.players) == 0 {
					existingGame.mu.Unlock()
					return ctx.Reply("No players in lobby yet! Type .wcg join to join first.")
				}
				if existingGame.lobbyTimer != nil {
					existingGame.lobbyTimer.Stop()
				}
				existingGame.mu.Unlock()
				existingGame.startGame(ctx)
				return nil
			}
			existingGame.mu.Unlock()
			return ctx.Reply("WCG game is already in progress!")
		}
	}

	// Default: if game already active, print status
	if exists {
		existingGame.mu.Lock()
		defer existingGame.mu.Unlock()
		if existingGame.state == wcgStateLobby {
			return ctx.Reply(fmt.Sprintf("WCG Lobby Open! (%d players)\nType .wcg join to join or .wcg start to begin!", len(existingGame.players)))
		}
		return ctx.Reply("A WCG game is already in progress in this chat!")
	}

	// Create new WCG lobby & send interactive menu
	hostLID := ctx.Sender.ToNonAD()
	hostMention, hostUser := ctx.ResolveMention(hostLID)
	hostTag := "@" + hostUser

	newGame := &wcgGame{
		chatKey:     chatKey,
		state:       wcgStateLobby,
		hostLID:     hostLID,
		hostMention: hostMention,
		hostTag:     hostTag,
		wordLength:  3,
		chatJID:     ctx.Chat,
		client:      ctx.Client,
	}

	// Host automatically joins
	newGame.players = append(newGame.players, &wcgPlayer{
		LID:        hostLID,
		MentionJID: hostMention,
		Tag:        hostTag,
		JoinedAt:   time.Now(),
	})

	wcgMu.Lock()
	wcgGames[chatKey] = newGame
	wcgMu.Unlock()

	// Start 30-second lobby countdown timer
	newGame.lobbyTimer = time.AfterFunc(30*time.Second, func() {
		newGame.mu.Lock()
		if newGame.state != wcgStateLobby {
			newGame.mu.Unlock()
			return
		}
		newGame.mu.Unlock()

		// Start game asynchronously
		cctx := &Context{
			Ctx:    ctx.Ctx,
			Client: ctx.Client,
			Chat:   ctx.Chat,
			Sender: ctx.Sender,
		}
		newGame.startGame(cctx)
	})

	// Try sending interactive message with buttons
	err := sendWCGInteractiveMenu(ctx, hostTag)
	if err != nil {
		// Fallback to text menu
		textMsg := fmt.Sprintf("🔤 WORD GUESSING GAME (WCG) 🔤\n\nHosted by: %s\n\n⏱️ Lobby is open for 30 SECONDS!\nType '.wcg join' to join\nType '.wcg start' to begin now\nType '.wcg lb' for Leaderboard", hostTag)
		return ctx.ReplyWithMentions(textMsg, []types.JID{hostMention})
	}

	return nil
}

func sendWCGInteractiveMenu(ctx *Context, hostTag string) error {
	msgVersion := int32(1)

	bodyText := fmt.Sprintf("🔤 WORD GUESSING GAME (WCG)\n\nHosted by %s\n\n⏱️ 30s Join Window Open!\nClick 'Join Match' or type '.wcg join' to play.\n\nRules:\n• Words progress from 3 to 16 letters\n• Turn time decreases as difficulty rises (30s → 6s)\n• Emojis & non-players are ignored\n• Win XP & climb performance ratings!", hostTag)

	msg := &waE2E.Message{
		DocumentWithCaptionMessage: &waE2E.FutureProofMessage{
			Message: &waE2E.Message{
				InteractiveMessage: &waE2E.InteractiveMessage{
					Body: &waE2E.InteractiveMessage_Body{
						Text: &bodyText,
					},
					Footer: &waE2E.InteractiveMessage_Footer{
						Text: new("WhatsRook Word Guessing Game"),
					},
					InteractiveMessage: &waE2E.InteractiveMessage_NativeFlowMessage_{
						NativeFlowMessage: &waE2E.InteractiveMessage_NativeFlowMessage{
							Buttons: []*waE2E.InteractiveMessage_NativeFlowMessage_NativeFlowButton{
								{
									Name:             new("quick_reply"),
									ButtonParamsJSON: new(`{"display_text":"🎮 Join Match","id":".wcg join"}`),
								},
								{
									Name:             new("quick_reply"),
									ButtonParamsJSON: new(`{"display_text":"▶️ Start Match","id":".wcg start"}`),
								},
								{
									Name:             new("quick_reply"),
									ButtonParamsJSON: new(`{"display_text":"🏆 Leaderboard","id":".wcg lb"}`),
								},
							},
							MessageVersion: &msgVersion,
						},
					},
				},
			},
		},
	}

	bizNode := waBinary.Node{
		Tag:   "biz",
		Attrs: waBinary.Attrs{},
		Content: []waBinary.Node{
			{
				Tag: "interactive",
				Attrs: waBinary.Attrs{
					"type": "native_flow",
					"v":    "1",
				},
				Content: []waBinary.Node{
					{
						Tag: "native_flow",
						Attrs: waBinary.Attrs{
							"v":    "9",
							"name": "mixed",
						},
					},
				},
			},
		},
	}

	extra := whatsmeow.SendRequestExtra{
		AdditionalNodes: &[]waBinary.Node{bizNode},
	}

	_, err := ctx.Client.SendMessage(ctx.Ctx, ctx.Chat, msg, extra)
	return err
}

func (g *wcgGame) startGame(ctx *Context) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.state == wcgStateInProgress {
		return
	}

	g.state = wcgStateInProgress
	g.gameStartTime = time.Now()
	g.wordLength = 3
	g.currentTurnIdx = 0

	active := g.getActivePlayers()
	if len(active) == 0 {
		wcgMu.Lock()
		delete(wcgGames, g.chatKey)
		wcgMu.Unlock()
		_ = ctx.Reply("WCG Match cancelled — no players joined the lobby.")
		return
	}

	slog.Debug("[WCG] Starting game", "chat", g.chatKey, "playersCount", len(active))

	var playerTags []string
	var mentions []types.JID
	for _, p := range active {
		playerTags = append(playerTags, p.Tag)
		mentions = append(mentions, p.MentionJID)
	}

	msg := fmt.Sprintf("🎮 WCG Match Started!\n\nPlayers (%d): %s\n\nStarting at Level 1 (3-Letter Words)!\nNon-players and turn-skipping input will be silently ignored.",
		len(active), strings.Join(playerTags, ", "))
	_ = ctx.ReplyWithMentions(msg, mentions)

	g.startTurn(ctx)
}

func (g *wcgGame) startTurn(ctx *Context) {
	active := g.getActivePlayers()
	if len(active) == 0 {
		g.finishGame(ctx)
		return
	}

	// Ensure currentTurnIdx points to valid active player
	if g.currentTurnIdx >= len(g.players) {
		g.currentTurnIdx = 0
	}
	for g.players[g.currentTurnIdx].Eliminated {
		g.currentTurnIdx = (g.currentTurnIdx + 1) % len(g.players)
	}

	currentPlayer := g.players[g.currentTurnIdx]

	word, scrambled := getRandomWord(g.wordLength)
	g.currentWord = word
	g.scrambledWord = scrambled
	g.turnStartTime = time.Now()

	// Dynamic time limit: 30s at level 3 down to 6s at level 16
	timeSec := getTurnTimeLimit(g.wordLength)
	timeDuration := time.Duration(timeSec) * time.Second

	msg := fmt.Sprintf("🔤 LEVEL %d (Word Length: %d)\n\nScrambled Word: %s\nTurn: %s\n⏱️ Time Limit: %d seconds!\n\nUnscramble and type the word!",
		g.wordLength-2, g.wordLength, g.scrambledWord, currentPlayer.Tag, timeSec)
	_ = ctx.ReplyWithMentions(msg, []types.JID{currentPlayer.MentionJID})

	// Set turn timer
	g.turnTimer = time.AfterFunc(timeDuration, func() {
		g.mu.Lock()
		defer g.mu.Unlock()

		if g.state != wcgStateInProgress {
			return
		}

		slog.Debug("[WCG] Turn timed out", "player", currentPlayer.Tag, "word", g.currentWord)
		currentPlayer.Eliminated = true

		cctx := &Context{
			Ctx:    ctx.Ctx,
			Client: g.client,
			Chat:   g.chatJID,
			Sender: ctx.Sender,
		}

		timeoutMsg := fmt.Sprintf("⏱️ Time's up for %s!\nThe word was: '%s'.\n%s has been eliminated!",
			currentPlayer.Tag, g.currentWord, currentPlayer.Tag)
		_ = cctx.ReplyWithMentions(timeoutMsg, []types.JID{currentPlayer.MentionJID})

		rem := g.getActivePlayers()
		if len(rem) <= 1 {
			g.finishGame(cctx)
			return
		}

		g.advanceTurn(cctx)
	})
}

func (g *wcgGame) advanceTurn(ctx *Context) {
	active := g.getActivePlayers()
	if len(active) <= 1 {
		g.finishGame(ctx)
		return
	}

	// Advance turn index to next non-eliminated player
	g.currentTurnIdx = (g.currentTurnIdx + 1) % len(g.players)
	for g.players[g.currentTurnIdx].Eliminated {
		g.currentTurnIdx = (g.currentTurnIdx + 1) % len(g.players)
	}

	g.startTurn(ctx)
}

func (g *wcgGame) finishGame(ctx *Context) {
	if g.turnTimer != nil {
		g.turnTimer.Stop()
	}

	active := g.getActivePlayers()

	var winner *wcgPlayer
	if len(active) == 1 {
		winner = active[0]
	}

	// Update DB stats & ratings for all players
	g.saveStatsAndXP(ctx, winner)

	var sb strings.Builder
	sb.WriteString("🏆 WCG MATCH OVER! 🏆\n\n")

	var mentions []types.JID

	if winner != nil {
		fmt.Fprintf(&sb, "🥇 Winner: %s (+100 Bonus XP!)\nTotal Score: %d pts\nCorrect Words: %d\n\n",
			winner.Tag, winner.Score, winner.CorrectGuesses)
		mentions = append(mentions, winner.MentionJID)
	} else {
		sb.WriteString("No winner — all players eliminated!\n\n")
	}

	sb.WriteString("Final Standings:\n")
	for i, p := range g.players {
		avgTimeSec := 0.0
		if p.GuessesCount > 0 {
			avgTimeSec = float64(p.TotalTimeMs) / float64(p.GuessesCount) / 1000.0
		}
		fmt.Fprintf(&sb, "%d. %s — %d pts (%d correct, avg %.1fs)\n", i+1, p.Tag, p.Score, p.CorrectGuesses, avgTimeSec)
		mentions = append(mentions, p.MentionJID)
	}

	wcgMu.Lock()
	delete(wcgGames, g.chatKey)
	wcgMu.Unlock()

	_ = ctx.ReplyWithMentions(sb.String(), mentions)
}

func (g *wcgGame) saveStatsAndXP(ctx *Context, winner *wcgPlayer) {
	s, ok := g.client.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return
	}
	db := s.GetDB()
	if db == nil {
		return
	}

	for _, p := range g.players {
		if p.LID.User == "" || p.LID.User == "whatsrook_bot" {
			continue
		}

		isWin := winner != nil && p.LID.User == winner.LID.User
		winInc := 0
		xpEarned := p.Score // base XP from word length points
		if isWin {
			winInc = 1
			xpEarned += 100 // Winner bonus
		} else {
			xpEarned += 10 // Participation XP
		}

		avgTimeMs := int64(0)
		if p.GuessesCount > 0 {
			avgTimeMs = p.TotalTimeMs / int64(p.GuessesCount)
		}

		// Performance Rating calculation
		// Base delta: +30 for win, -15 for loss
		// Speed bonus: up to +20 if fast (< 3s average)
		ratingDelta := -15
		if isWin {
			ratingDelta = 30
		}
		if p.CorrectGuesses > 0 && avgTimeMs > 0 {
			if avgTimeMs < 3000 {
				ratingDelta += 15
			} else if avgTimeMs > 10000 {
				ratingDelta -= 10
			}
		}
		// Poor performance penalty (0 correct guesses)
		if p.CorrectGuesses == 0 {
			ratingDelta -= 15
			if xpEarned > 20 {
				xpEarned = 5
			}
		}

		cleanJID := p.MentionJID.ToNonAD().String()

		_, _ = db.Exec(ctx.Ctx, `INSERT INTO bot_user_xp (user_jid, xp, wcg_wins, wcg_games, wcg_rating)
			VALUES ($1, $2, $3, 1, $4)
			ON CONFLICT(user_jid) DO UPDATE SET
				xp = MAX(0, xp + EXCLUDED.xp),
				wcg_wins = wcg_wins + EXCLUDED.wcg_wins,
				wcg_games = wcg_games + 1,
				wcg_rating = MAX(100, wcg_rating + $5)`,
			cleanJID, xpEarned, winInc, 1000+ratingDelta, ratingDelta)
	}
}

func (g *wcgGame) getActivePlayers() []*wcgPlayer {
	var active []*wcgPlayer
	for _, p := range g.players {
		if !p.Eliminated {
			active = append(active, p)
		}
	}
	return active
}

func (g *wcgGame) findPlayerIndex(lid types.JID) int {
	for i, p := range g.players {
		if p.LID.User == lid.User {
			return i
		}
	}
	return -1
}

// getTurnTimeLimit calculates turn duration: Level 3 (3-letter) = 30s, Level 16 (16-letter) = 6s.
func getTurnTimeLimit(level int) int {
	if level <= 3 {
		return 30
	}
	if level >= 16 {
		return 6
	}
	// Linear interpolation: level 3 -> 30s, level 16 -> 6s
	t := 30 - int(float64(level-3)*(24.0/13.0))
	if t < 6 {
		return 6
	}
	if t > 30 {
		return 30
	}
	return t
}

// GetCXPTitle maps cumulative XP (CXP) to player titles.
func GetCXPTitle(xp int) string {
	switch {
	case xp >= 12000:
		return "👑 Legendary Master"
	case xp >= 7000:
		return "🌟 Legend"
	case xp >= 3500:
		return "⚡ Prolific"
	case xp >= 1500:
		return "🔥 Master"
	case xp >= 500:
		return "⚔️ Pro"
	case xp >= 100:
		return "🌱 Beginner"
	default:
		return "🐣 Novice"
	}
}

func handleWCGLeaderboard(ctx *Context) error {
	s, ok := ctx.Client.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return ctx.Reply("Leaderboard store unavailable.")
	}
	db := s.GetDB()
	if db == nil {
		return ctx.Reply("Database connection unavailable.")
	}

	rows, err := db.Query(ctx.Ctx, `SELECT user_jid, xp, wcg_wins, wcg_games, wcg_rating FROM bot_user_xp ORDER BY xp DESC LIMIT 10`)
	if err != nil {
		return ctx.Reply("Failed to fetch WCG leaderboard.")
	}
	defer rows.Close()

	type wcgLBEntry struct {
		tag    string
		xp     int
		title  string
		wins   int
		games  int
		rating int
	}

	var entries []wcgLBEntry
	var mentions []types.JID

	for rows.Next() {
		var jidStr string
		var xp, wins, games, rating int
		if err := rows.Scan(&jidStr, &xp, &wins, &games, &rating); err == nil {
			if rating == 0 {
				rating = 1000
			}
			parsed, pErr := types.ParseJID(jidStr)
			if pErr == nil {
				tag, resolved := ctx.FormatMention(parsed)
				entries = append(entries, wcgLBEntry{
					tag:    tag,
					xp:     xp,
					title:  GetCXPTitle(xp),
					wins:   wins,
					games:  games,
					rating: rating,
				})
				mentions = append(mentions, resolved)
			}
		}
	}

	if len(entries) == 0 {
		return ctx.Reply("No players on the WCG leaderboard yet! Type .wcg to start a game and earn XP.")
	}

	var sb strings.Builder
	sb.WriteString("🏆 WCG LEADERBOARD & CUMULATIVE XP (CXP) 🏆\n\n")

	for i, e := range entries {
		fmt.Fprintf(&sb, "%d. %s — %s (%d CXP)\n   Rating: %d | Wins: %d/%d\n\n",
			i+1, e.tag, e.title, e.xp, e.rating, e.wins, e.games)
	}

	return ctx.ReplyWithMentions(strings.TrimSpace(sb.String()), mentions)
}

// isPureEmoji returns true if the input text consists solely of emoji characters.
func isPureEmoji(s string) bool {
	hasRune := false
	for _, r := range strings.TrimSpace(s) {
		if unicode.IsSpace(r) {
			continue
		}
		hasRune = true
		// Range checks for common emoji blocks
		if !isEmojiRune(r) {
			return false
		}
	}
	return hasRune
}

func isEmojiRune(r rune) bool {
	return (r >= 0x1F600 && r <= 0x1F64F) || // Emoticons
		(r >= 0x1F300 && r <= 0x1F5FF) || // Misc Symbols and Pictographs
		(r >= 0x1F680 && r <= 0x1F6FF) || // Transport and Map
		(r >= 0x1F700 && r <= 0x1F77F) || // Alchemical Symbols
		(r >= 0x1F780 && r <= 0x1F7FF) || // Geometric Shapes Extended
		(r >= 0x1F800 && r <= 0x1F8FF) || // Supplemental Arrows-C
		(r >= 0x1F900 && r <= 0x1F9FF) || // Supplemental Symbols and Pictographs
		(r >= 0x1FA00 && r <= 0x1FA6F) || // Chess Symbols
		(r >= 0x1FA70 && r <= 0x1FAFF) || // Symbols and Pictographs Extended-A
		(r >= 0x2600 && r <= 0x26FF) || // Misc Symbols
		(r >= 0x2700 && r <= 0x27BF) || // Dingbats
		(r >= 0xFE00 && r <= 0xFE0F) || // Variation Selectors
		(r >= 0x1F1E6 && r <= 0x1F1FF) // Regional Indicator Symbols
}
