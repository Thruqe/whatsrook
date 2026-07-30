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

	"go.mau.fi/whatsmeow/types"
)

func init() {
	Register(&Command{
		Name:        "unscramble",
		Aliases:     []string{"unscrambleword", "wordunscramble"},
		Description: "Unscramble word game with 30s lobby, dynamic time limits, performance ratings & XP",
		Category:    "games",
		IsPublic:    true,
		Handler:     handleUnscramble,
	})
}

// HandleUnscrambleInput intercepts non-prefix text messages in a chat where an Unscramble game is active.
// Returns true if the message was handled/swallowed by Unscramble.
func HandleUnscrambleInput(ctx *Context, text string) bool {
	chatKey := ctx.Chat.String()

	game := utils.GetUnscrambleGame(chatKey)
	if game == nil {
		return false
	}

	game.Mu.Lock()

	// In lobby phase, ignore normal chat messages
	if game.State == utils.UnscrambleStateLobby {
		game.Mu.Unlock()
		return false
	}

	senderLID := ctx.Sender.ToNonAD()

	// Check if message is pure emoji or empty -> ignore without reply
	if isPureEmoji(text) || strings.TrimSpace(text) == "" {
		slog.Debug("[Unscramble] Ignored emoji/empty input", "chat", chatKey, "sender", senderLID.String())
		game.Mu.Unlock()
		return true
	}

	// Check if sender is in the game -> if not, let dispatch handle potential commands
	pIdx := game.FindPlayerIndex(senderLID)
	if pIdx == -1 {
		slog.Debug("[Unscramble] Ignored input from non-player", "chat", chatKey, "sender", senderLID.String())
		game.Mu.Unlock()
		return false
	}

	// Check if it's the sender's turn -> if not, let dispatch handle potential commands
	activePlayers := game.GetActivePlayers()
	if len(activePlayers) == 0 {
		game.Mu.Unlock()
		return false
	}

	currentTurnPlayer := game.Players[game.CurrentTurnIdx]
	if currentTurnPlayer.LID.User != senderLID.User {
		slog.Debug("[Unscramble] Ignored input from player whose turn it is not", "chat", chatKey, "sender", senderLID.String())
		game.Mu.Unlock()
		return false
	}

	// Process the guess (release lock first, ProcessGuess needs its own lock)
	game.Mu.Unlock()
	correct, gameOver, winner, currentPlayer, elapsed := game.ProcessGuess(text, senderLID)

	if correct {
		msg := fmt.Sprintf("Correct! %s guessed '%s' in %.1fs! (+%d pts)\n\nAdvancing to the next level!",
			currentPlayer.Tag, game.CurrentWord, elapsed.Seconds(), game.WordLength*10)
		_ = ctx.ReplyWithMentions(msg, []types.JID{currentPlayer.MentionJID})

		if gameOver {
			finishUnscrambleGame(ctx, game, winner)
			return true
		}

		// Start next turn
		startUnscrambleTurn(ctx, game)
		return true
	}

	// Wrong guess
	msg := fmt.Sprintf("Incorrect guess by %s!\nThe correct word was: '%s'.\n%s has been eliminated from this match!",
		currentPlayer.Tag, game.CurrentWord, currentPlayer.Tag)
	_ = ctx.ReplyWithMentions(msg, []types.JID{currentPlayer.MentionJID})

	if gameOver {
		finishUnscrambleGame(ctx, game, winner)
		return true
	}

	// Start next turn
	startUnscrambleTurn(ctx, game)
	return true
}

func handleUnscramble(ctx *Context) error {
	chatKey := ctx.Chat.String()

	existingGame := utils.GetUnscrambleGame(chatKey)

	arg0 := ""
	if len(ctx.Args) > 0 {
		arg0 = strings.ToLower(ctx.Args[0])
	}

	if arg0 == "lb" || arg0 == "leaderboard" {
		return handleUnscrambleLeaderboard(ctx)
	}

	if arg0 == "cancel" || arg0 == "stop" {
		if existingGame == nil {
			return ctx.Reply("No active Unscramble game to cancel.")
		}
		existingGame.StopTimers()
		utils.DeleteUnscrambleGame(chatKey)
		return ctx.Reply("Unscramble Game cancelled.")
	}

	// Join sub-command
	if arg0 == "join" {
		if existingGame == nil {
			return ctx.Reply("No Unscramble game lobby open. Type .unscramble to start one!")
		}

		existingGame.Mu.Lock()
		if existingGame.State != utils.UnscrambleStateLobby {
			existingGame.Mu.Unlock()
			return ctx.Reply("Unscramble game is already in progress!")
		}

		senderLID := ctx.Sender.ToNonAD()
		if existingGame.FindPlayerIndex(senderLID) != -1 {
			existingGame.Mu.Unlock()
			return ctx.Reply("You are already in the Unscramble lobby!")
		}
		existingGame.Mu.Unlock()

		mentionJID, username := ctx.ResolveMention(senderLID)
		tag := "@" + username
		if !existingGame.AddPlayer(senderLID, mentionJID, tag) {
			return ctx.Reply("Failed to join. Game may have started.")
		}

		msg := fmt.Sprintf("%s joined the Unscramble match! (%d players in lobby)\nType .unscramble start to begin immediately or wait for timer.", tag, len(existingGame.Players))
		return ctx.ReplyWithMentions(msg, []types.JID{mentionJID})
	}

	// Start sub-command
	if arg0 == "start" || arg0 == "create" {
		if existingGame != nil {
			existingGame.Mu.Lock()
			if existingGame.State == utils.UnscrambleStateLobby {
				if len(existingGame.Players) == 0 {
					existingGame.Mu.Unlock()
					return ctx.Reply("No players in lobby yet! Type .unscramble join to join first.")
				}
				if existingGame.LobbyTimer != nil {
					existingGame.LobbyTimer.Stop()
				}
				existingGame.Mu.Unlock()
				startUnscrambleGame(ctx, existingGame)
				return nil
			}
			existingGame.Mu.Unlock()
			return ctx.Reply("Unscramble game is already in progress!")
		}
	}

	// Default: if game already active, print status
	if existingGame != nil {
		existingGame.Mu.Lock()
		defer existingGame.Mu.Unlock()
		if existingGame.State == utils.UnscrambleStateLobby {
			return ctx.Reply(fmt.Sprintf("Unscramble Lobby Open! (%d players)\nType .unscramble join to join or .unscramble start to begin!", len(existingGame.Players)))
		}
		return ctx.Reply("An Unscramble game is already in progress in this chat!")
	}

	// Create new Unscramble lobby
	hostLID := ctx.Sender.ToNonAD()
	hostMention, hostUser := ctx.ResolveMention(hostLID)
	hostTag := "@" + hostUser

	newGame := utils.CreateUnscrambleGame(chatKey, hostLID, hostMention, hostTag, ctx.Chat, ctx.Client)

	// Start 30-second lobby countdown timer
	timer := time.AfterFunc(30*time.Second, func() {
		newGame.Mu.Lock()
		if newGame.State != utils.UnscrambleStateLobby {
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
		startUnscrambleGame(cctx, newGame)
	})
	newGame.SetLobbyTimer(timer)

	// Try sending interactive message with buttons
	err := sendUnscrambleInteractiveMenu(ctx, hostTag)
	if err != nil {
		textMsg := fmt.Sprintf("UNSCRAMBLE GAME\n\nHosted by: %s\n\nLobby is open for 30 SECONDS!\nType '.unscramble join' to join\nType '.unscramble start' to begin now\nType '.unscramble lb' for Leaderboard", hostTag)
		return ctx.ReplyWithMentions(textMsg, []types.JID{hostMention})
	}

	return nil
}

func startUnscrambleGame(ctx *Context, game *utils.UnscrambleGame) {
	if !game.StartGame() {
		_ = ctx.Reply("Unscramble Match cancelled — no players joined the lobby.")
		return
	}

	active := game.GetActivePlayers()

	slog.Debug("[Unscramble] Starting game", "chat", game.ChatKey, "playersCount", len(active))

	var playerTags []string
	var mentions []types.JID
	for _, p := range active {
		playerTags = append(playerTags, p.Tag)
		mentions = append(mentions, p.MentionJID)
	}

	msg := fmt.Sprintf("Unscramble Match Started!\n\nPlayers (%d): %s\n\nStarting at Level 1 (3-Letter Words)!\nNon-players and turn-skipping input will be silently ignored.",
		len(active), strings.Join(playerTags, ", "))
	_ = ctx.ReplyWithMentions(msg, mentions)

	startUnscrambleTurn(ctx, game)
}

func startUnscrambleTurn(ctx *Context, game *utils.UnscrambleGame) {
	scrambled, timeSec, currentPlayer := game.StartTurn()
	if currentPlayer == nil {
		winner, _ := game.FinishGame()
		finishUnscrambleGame(ctx, game, winner)
		return
	}

	msg := fmt.Sprintf("LEVEL %d (Word Length: %d)\n\nScrambled Word: %s\nTurn: %s\nTime Limit: %d seconds!\n\nUnscramble and type the word!",
		game.WordLength-2, game.WordLength, scrambled, currentPlayer.Tag, timeSec)
	_ = ctx.ReplyWithMentions(msg, []types.JID{currentPlayer.MentionJID})

	// Set turn timer
	timeDuration := time.Duration(timeSec) * time.Second
	timer := time.AfterFunc(timeDuration, func() {
		game.Mu.Lock()
		inProgress := game.State == utils.UnscrambleStateInProgress
		game.Mu.Unlock()

		if !inProgress {
			return
		}

		slog.Debug("[Unscramble] Turn timed out", "player", currentPlayer.Tag, "word", game.CurrentWord)

		cctx := &Context{
			Ctx:    ctx.Ctx,
			Client: game.Client,
			Chat:   game.ChatJID,
			Sender: ctx.Sender,
		}

		timeoutMsg := fmt.Sprintf("Time's up for %s!\nThe word was: '%s'.\n%s has been eliminated!",
			currentPlayer.Tag, game.CurrentWord, currentPlayer.Tag)
		_ = cctx.ReplyWithMentions(timeoutMsg, []types.JID{currentPlayer.MentionJID})

		game.StopTimers()
		gameOver, winner := game.EliminateCurrentPlayer()

		if gameOver {
			finishUnscrambleGame(cctx, game, winner)
			return
		}

		startUnscrambleTurn(cctx, game)
	})
	game.SetTurnTimer(timer)
}

func finishUnscrambleGame(ctx *Context, game *utils.UnscrambleGame, winner *utils.UnscramblePlayer) {
	finalWinner, standings := game.FinishGame()
	if finalWinner != nil {
		winner = finalWinner
	}

	// Save stats to DB
	saveUnscrambleStats(ctx, game, winner)

	var sb strings.Builder
	sb.WriteString("UNSCRAMBLE MATCH OVER!\n\n")

	var mentions []types.JID

	if winner != nil {
		fmt.Fprintf(&sb, "1. Winner: %s (+100 Bonus XP!)\nTotal Score: %d pts\nCorrect Words: %d\n\n",
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

func saveUnscrambleStats(ctx *Context, game *utils.UnscrambleGame, winner *utils.UnscramblePlayer) {
	// Scores in DM games are not added to group leaderboards
	if game.ChatJID.Server != "g.us" {
		return
	}

	s, ok := game.Client.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return
	}
	db := s.GetDB()
	if db == nil {
		return
	}

	groupJID := game.ChatJID.ToNonAD().String()

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

		_, _ = db.Exec(ctx.Ctx, `INSERT INTO bot_group_user_xp (group_jid, user_jid, xp, wcg_wins, wcg_games, wcg_rating)
			VALUES ($1, $2, $3, $4, 1, $5)
			ON CONFLICT(group_jid, user_jid) DO UPDATE SET
				xp = MAX(0, bot_group_user_xp.xp + EXCLUDED.xp),
				wcg_wins = bot_group_user_xp.wcg_wins + EXCLUDED.wcg_wins,
				wcg_games = bot_group_user_xp.wcg_games + 1,
				wcg_rating = MAX(100, bot_group_user_xp.wcg_rating + $6)`,
			groupJID, cleanJID, xpEarned, winInc, 1000+ratingDelta, ratingDelta)
	}
}

func handleUnscrambleLeaderboard(ctx *Context) error {
	chatKey := ctx.Chat.String()

	game := utils.GetUnscrambleGame(chatKey)
	if game == nil {
		return ctx.Reply("No active Unscramble game in this chat. Start one with .unscramble")
	}

	sorted := game.GetSortedPlayers()
	if len(sorted) == 0 {
		return ctx.Reply("No players in the current Unscramble game.")
	}

	game.Mu.Lock()
	state := game.State
	game.Mu.Unlock()

	var sb strings.Builder
	if state == utils.UnscrambleStateLobby {
		sb.WriteString("UNSCRAMBLE LOBBY STANDINGS\n\n")
	} else {
		sb.WriteString("UNSCRAMBLE MATCH STANDINGS\n\n")
	}

	var mentions []types.JID
	for i, p := range sorted {
		status := ""
		if p.Eliminated {
			status = " (Eliminated)"
		} else if state == utils.UnscrambleStateInProgress {
			status = " (Active)"
		}
		fmt.Fprintf(&sb, "%d. %s — %d pts (%d correct)%s\n", i+1, p.Tag, p.Score, p.CorrectGuesses, status)
		mentions = append(mentions, p.MentionJID)
	}

	return ctx.ReplyWithMentions(strings.TrimSpace(sb.String()), mentions)
}

func sendUnscrambleInteractiveMenu(ctx *Context, hostTag string) error {
	p := ctx.GetPrefix()
	bodyText := fmt.Sprintf("UNSCRAMBLE GAME\n\nHosted by %s\n\n30s Join Window Open!\nClick 'Join Match' or type '%sunscramble join' to play.\n\nRules:\n- Words progress from 3 to 16 letters\n- Turn time decreases as difficulty rises (30s -> 6s)\n- Non-players are ignored\n- Win XP and climb performance ratings!", hostTag, p)

	buttons := []struct{ ID, Text string }{
		{ID: p + "unscramble join", Text: "Join Match"},
		{ID: p + "unscramble start", Text: "Start Match"},
		{ID: p + "unscramble lb", Text: "Leaderboard"},
	}

	return sendInteractiveButtons(ctx, bodyText, "WhatsRook Unscramble Game", buttons)
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
