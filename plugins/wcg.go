// Word Guessing Game (WCG) – Command handler using utils/wcg_game.go engine.
package commands

import (
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode"

	"whatsrook/store/sqlstore"
	"whatsrook/utils"

	"go.mau.fi/whatsmeow"
	waBinary "go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
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

// HandleWCGInput intercepts non-prefix text messages in a chat where a WCG game is active.
// Returns true if the message was handled/swallowed by WCG.
func HandleWCGInput(ctx *Context, text string) bool {
	chatKey := ctx.Chat.String()

	game := utils.GetWCGGame(chatKey)
	if game == nil {
		return false
	}

	game.Mu.Lock()

	// In lobby phase, ignore normal chat messages
	if game.State == utils.WCGStateLobby {
		game.Mu.Unlock()
		return false
	}

	senderLID := ctx.Sender.ToNonAD()

	// Check if message is pure emoji or empty -> ignore without reply
	if isPureEmoji(text) || strings.TrimSpace(text) == "" {
		slog.Debug("[WCG] Ignored emoji/empty input", "chat", chatKey, "sender", senderLID.String())
		game.Mu.Unlock()
		return true
	}

	// Check if sender is in the game
	pIdx := game.FindPlayerIndex(senderLID)
	if pIdx == -1 {
		slog.Debug("[WCG] Ignored input from non-player", "chat", chatKey, "sender", senderLID.String())
		game.Mu.Unlock()
		return true
	}

	// Check if it's the sender's turn
	activePlayers := game.GetActivePlayers()
	if len(activePlayers) == 0 {
		game.Mu.Unlock()
		return true
	}

	currentTurnPlayer := game.Players[game.CurrentTurnIdx]
	if currentTurnPlayer.LID.User != senderLID.User {
		slog.Debug("[WCG] Ignored input from player whose turn it is not", "chat", chatKey, "sender", senderLID.String())
		game.Mu.Unlock()
		return true
	}

	// Process the guess (release lock first, ProcessGuess needs its own lock)
	game.Mu.Unlock()
	correct, gameOver, winner, currentPlayer, elapsed := game.ProcessGuess(text, senderLID)
	game.Mu.Lock()

	if correct {
		msg := fmt.Sprintf("🎉 Correct! %s guessed '%s' in %.1fs! (+%d pts)\n\nAdvancing to the next level!",
			currentPlayer.Tag, game.CurrentWord, elapsed.Seconds(), game.WordLength*10)
		_ = ctx.ReplyWithMentions(msg, []types.JID{currentPlayer.MentionJID})

		if gameOver {
			game.Mu.Unlock()
			finishWCGGame(ctx, game, winner)
			return true
		}

		// Start next turn
		game.Mu.Unlock()
		startWCGTurn(ctx, game)
		return true
	}

	// Wrong guess
	msg := fmt.Sprintf("❌ Incorrect guess by %s!\nThe correct word was: '%s'.\n%s has been eliminated from this match!",
		currentPlayer.Tag, game.CurrentWord, currentPlayer.Tag)
	_ = ctx.ReplyWithMentions(msg, []types.JID{currentPlayer.MentionJID})

	if gameOver {
		game.Mu.Unlock()
		finishWCGGame(ctx, game, winner)
		return true
	}

	// Start next turn
	game.Mu.Unlock()
	startWCGTurn(ctx, game)
	return true
}

func handleWCG(ctx *Context) error {
	chatKey := ctx.Chat.String()

	existingGame := utils.GetWCGGame(chatKey)

	arg0 := ""
	if len(ctx.Args) > 0 {
		arg0 = strings.ToLower(ctx.Args[0])
	}

	if arg0 == "lb" || arg0 == "leaderboard" {
		return handleWCGLeaderboard(ctx)
	}

	if arg0 == "cancel" || arg0 == "stop" {
		if existingGame == nil {
			return ctx.Reply("No active WCG game to cancel.")
		}
		existingGame.StopTimers()
		utils.DeleteWCGGame(chatKey)
		return ctx.Reply("Word Guessing Game cancelled.")
	}

	// Join sub-command
	if arg0 == "join" {
		if existingGame == nil {
			return ctx.Reply("No WCG game lobby open. Type .wcg to start one!")
		}

		existingGame.Mu.Lock()
		if existingGame.State != utils.WCGStateLobby {
			existingGame.Mu.Unlock()
			return ctx.Reply("WCG game is already in progress!")
		}

		senderLID := ctx.Sender.ToNonAD()
		if existingGame.FindPlayerIndex(senderLID) != -1 {
			existingGame.Mu.Unlock()
			return ctx.Reply("You are already in the WCG lobby!")
		}
		existingGame.Mu.Unlock()

		mentionJID, username := ctx.ResolveMention(senderLID)
		tag := "@" + username
		if !existingGame.AddPlayer(senderLID, mentionJID, tag) {
			return ctx.Reply("Failed to join. Game may have started.")
		}

		msg := fmt.Sprintf("✅ %s joined the WCG match! (%d players in lobby)\nType .wcg start to begin immediately or wait for timer.", tag, len(existingGame.Players))
		return ctx.ReplyWithMentions(msg, []types.JID{mentionJID})
	}

	// Start sub-command
	if arg0 == "start" || arg0 == "create" {
		if existingGame != nil {
			existingGame.Mu.Lock()
			if existingGame.State == utils.WCGStateLobby {
				if len(existingGame.Players) == 0 {
					existingGame.Mu.Unlock()
					return ctx.Reply("No players in lobby yet! Type .wcg join to join first.")
				}
				if existingGame.LobbyTimer != nil {
					existingGame.LobbyTimer.Stop()
				}
				existingGame.Mu.Unlock()
				startWCGGame(ctx, existingGame)
				return nil
			}
			existingGame.Mu.Unlock()
			return ctx.Reply("WCG game is already in progress!")
		}
	}

	// Default: if game already active, print status
	if existingGame != nil {
		existingGame.Mu.Lock()
		defer existingGame.Mu.Unlock()
		if existingGame.State == utils.WCGStateLobby {
			return ctx.Reply(fmt.Sprintf("WCG Lobby Open! (%d players)\nType .wcg join to join or .wcg start to begin!", len(existingGame.Players)))
		}
		return ctx.Reply("A WCG game is already in progress in this chat!")
	}

	// Create new WCG lobby
	hostLID := ctx.Sender.ToNonAD()
	hostMention, hostUser := ctx.ResolveMention(hostLID)
	hostTag := "@" + hostUser

	newGame := utils.CreateWCGGame(chatKey, hostLID, hostMention, hostTag, ctx.Chat, ctx.Client)

	// Start 30-second lobby countdown timer
	timer := time.AfterFunc(30*time.Second, func() {
		newGame.Mu.Lock()
		if newGame.State != utils.WCGStateLobby {
			newGame.Mu.Unlock()
			return
		}
		newGame.Mu.Unlock()

		cctx := &Context{
			Ctx:    ctx.Ctx,
			Client: ctx.Client,
			Chat:   ctx.Chat,
			Sender: ctx.Sender,
		}
		startWCGGame(cctx, newGame)
	})
	newGame.SetLobbyTimer(timer)

	// Try sending interactive message with buttons
	err := sendWCGInteractiveMenu(ctx, hostTag)
	if err != nil {
		textMsg := fmt.Sprintf("🔤 WORD GUESSING GAME (WCG) 🔤\n\nHosted by: %s\n\n⏱️ Lobby is open for 30 SECONDS!\nType '.wcg join' to join\nType '.wcg start' to begin now\nType '.wcg lb' for Leaderboard", hostTag)
		return ctx.ReplyWithMentions(textMsg, []types.JID{hostMention})
	}

	return nil
}

func startWCGGame(ctx *Context, game *utils.WCGGame) {
	if !game.StartGame() {
		_ = ctx.Reply("WCG Match cancelled — no players joined the lobby.")
		return
	}

	active := game.GetActivePlayers()

	slog.Debug("[WCG] Starting game", "chat", game.ChatKey, "playersCount", len(active))

	var playerTags []string
	var mentions []types.JID
	for _, p := range active {
		playerTags = append(playerTags, p.Tag)
		mentions = append(mentions, p.MentionJID)
	}

	msg := fmt.Sprintf("🎮 WCG Match Started!\n\nPlayers (%d): %s\n\nStarting at Level 1 (3-Letter Words)!\nNon-players and turn-skipping input will be silently ignored.",
		len(active), strings.Join(playerTags, ", "))
	_ = ctx.ReplyWithMentions(msg, mentions)

	startWCGTurn(ctx, game)
}

func startWCGTurn(ctx *Context, game *utils.WCGGame) {
	scrambled, timeSec, currentPlayer := game.StartTurn()
	if currentPlayer == nil {
		winner, _ := game.FinishGame()
		finishWCGGame(ctx, game, winner)
		return
	}

	msg := fmt.Sprintf("🔤 LEVEL %d (Word Length: %d)\n\nScrambled Word: %s\nTurn: %s\n⏱️ Time Limit: %d seconds!\n\nUnscramble and type the word!",
		game.WordLength-2, game.WordLength, scrambled, currentPlayer.Tag, timeSec)
	_ = ctx.ReplyWithMentions(msg, []types.JID{currentPlayer.MentionJID})

	// Set turn timer
	timeDuration := time.Duration(timeSec) * time.Second
	timer := time.AfterFunc(timeDuration, func() {
		game.Mu.Lock()
		if game.State != utils.WCGStateInProgress {
			game.Mu.Unlock()
			return
		}

		slog.Debug("[WCG] Turn timed out", "player", currentPlayer.Tag, "word", game.CurrentWord)

		cctx := &Context{
			Ctx:    ctx.Ctx,
			Client: game.Client,
			Chat:   game.ChatJID,
			Sender: ctx.Sender,
		}

		timeoutMsg := fmt.Sprintf("⏱️ Time's up for %s!\nThe word was: '%s'.\n%s has been eliminated!",
			currentPlayer.Tag, game.CurrentWord, currentPlayer.Tag)
		_ = cctx.ReplyWithMentions(timeoutMsg, []types.JID{currentPlayer.MentionJID})

		gameOver, winner := game.EliminateCurrentPlayer()
		game.Mu.Unlock()

		if gameOver {
			finishWCGGame(cctx, game, winner)
			return
		}

		startWCGTurn(cctx, game)
	})
	game.SetTurnTimer(timer)
}

func finishWCGGame(ctx *Context, game *utils.WCGGame, winner *utils.WCGPlayer) {
	finalWinner, standings := game.FinishGame()
	if finalWinner != nil {
		winner = finalWinner
	}

	// Save stats to DB
	saveWCGStats(ctx, game, winner)

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
	for i, p := range standings {
		avgTimeSec := 0.0
		if p.GuessesCount > 0 {
			avgTimeSec = float64(p.TotalTimeMs) / float64(p.GuessesCount) / 1000.0
		}
		fmt.Fprintf(&sb, "%d. %s — %d pts (%d correct, avg %.1fs)\n", i+1, p.Tag, p.Score, p.CorrectGuesses, avgTimeSec)
		mentions = append(mentions, p.MentionJID)
	}

	_ = ctx.ReplyWithMentions(sb.String(), mentions)
}

func saveWCGStats(ctx *Context, game *utils.WCGGame, winner *utils.WCGPlayer) {
	s, ok := game.Client.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return
	}
	db := s.GetDB()
	if db == nil {
		return
	}

	for _, p := range game.Players {
		if p.LID.User == "" || p.LID.User == "whatsrook_bot" {
			continue
		}

		isWin := winner != nil && p.LID.User == winner.LID.User
		winInc := 0
		xpEarned := p.Score
		if isWin {
			winInc = 1
			xpEarned += 100
		} else {
			xpEarned += 10
		}

		avgTimeMs := int64(0)
		if p.GuessesCount > 0 {
			avgTimeMs = p.TotalTimeMs / int64(p.GuessesCount)
		}

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

func handleWCGLeaderboard(ctx *Context) error {
	chatKey := ctx.Chat.String()

	game := utils.GetWCGGame(chatKey)
	if game == nil {
		return ctx.Reply("No active WCG game in this chat. Start one with .wcg")
	}

	sorted := game.GetSortedPlayers()
	if len(sorted) == 0 {
		return ctx.Reply("No players in the current WCG game.")
	}

	game.Mu.Lock()
	state := game.State
	game.Mu.Unlock()

	var sb strings.Builder
	if state == utils.WCGStateLobby {
		sb.WriteString("📋 WCG LOBBY STANDINGS\n\n")
	} else {
		sb.WriteString("📊 WCG MATCH STANDINGS\n\n")
	}

	var mentions []types.JID
	for i, p := range sorted {
		status := ""
		if p.Eliminated {
			status = " ❌ Eliminated"
		} else if state == utils.WCGStateInProgress {
			status = " ✅ Active"
		}
		fmt.Fprintf(&sb, "%d. %s — %d pts (%d correct)%s\n", i+1, p.Tag, p.Score, p.CorrectGuesses, status)
		mentions = append(mentions, p.MentionJID)
	}

	return ctx.ReplyWithMentions(strings.TrimSpace(sb.String()), mentions)
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

// isPureEmoji returns true if the input text consists solely of emoji characters.
func isPureEmoji(s string) bool {
	hasRune := false
	for _, r := range strings.TrimSpace(s) {
		if unicode.IsSpace(r) {
			continue
		}
		hasRune = true
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
