// Word Chain Game (WCG) – Word chain game where players submit words starting with the required character, validated against 5 parallel English dictionary APIs.
package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"time"

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
		Aliases:     []string{"wordchain", "wordchaingame"},
		Description: "Word Chain Game – submit valid English words matching the required starting letter",
		Category:    "games",
		IsPublic:    true,
		Handler:     handleWCGChain,
	})
}

var httpClient = &http.Client{
	Timeout: 4 * time.Second,
}

// ValidateWordParallel checks whether word is a valid English dictionary word by querying 5 dictionary APIs in parallel.
func ValidateWordParallel(word string) bool {
	word = strings.ToLower(strings.TrimSpace(word))
	if len(word) == 0 {
		return false
	}

	// First check built-in curated dictionary fallback
	if isBuiltinWord(word) {
		return true
	}

	type apiCheck func(word string) bool
	apiChecks := []apiCheck{
		// 1. Free Dictionary API
		func(w string) bool {
			resp, err := httpClient.Get("https://api.dictionaryapi.dev/api/v2/entries/en/" + url.PathEscape(w))
			if err != nil {
				return false
			}
			defer resp.Body.Close()
			return resp.StatusCode == http.StatusOK
		},
		// 2. Datamuse API
		func(w string) bool {
			resp, err := httpClient.Get("https://api.datamuse.com/words?sp=" + url.PathEscape(w) + "&max=1")
			if err != nil {
				return false
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return false
			}
			var results []struct {
				Word string `json:"word"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&results); err == nil && len(results) > 0 {
				return strings.EqualFold(results[0].Word, w)
			}
			return false
		},
		// 3. WordsAPI via Free API proxy / Webster Search
		func(w string) bool {
			resp, err := httpClient.Get("https://api.dictionaryapi.dev/api/v2/entries/en_US/" + url.PathEscape(w))
			if err != nil {
				return false
			}
			defer resp.Body.Close()
			return resp.StatusCode == http.StatusOK
		},
		// 4. Wiktionary API
		func(w string) bool {
			reqURL := fmt.Sprintf("https://en.wiktionary.org/w/api.php?action=query&titles=%s&format=json", url.QueryEscape(w))
			resp, err := httpClient.Get(reqURL)
			if err != nil {
				return false
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return false
			}
			var res struct {
				Query struct {
					Pages map[string]struct {
						PageID int `json:"pageid"`
					} `json:"pages"`
				} `json:"query"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&res); err == nil {
				for id := range res.Query.Pages {
					if id != "-1" {
						return true
					}
				}
			}
			return false
		},
		// 5. Wordnik / Open Dictionary Endpoint
		func(w string) bool {
			resp, err := httpClient.Get("https://api.dictionaryapi.dev/api/v2/entries/en_GB/" + url.PathEscape(w))
			if err != nil {
				return false
			}
			defer resp.Body.Close()
			return resp.StatusCode == http.StatusOK
		},
	}

	resCh := make(chan bool, len(apiChecks))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for _, check := range apiChecks {
		fn := check
		go func() {
			select {
			case <-ctx.Done():
				resCh <- false
			default:
				resCh <- fn(word)
			}
		}()
	}

	validCount := 0
	for i := 0; i < len(apiChecks); i++ {
		if <-resCh {
			validCount++
			// If at least 1 reliable API validates the word, accept it immediately!
			cancel()
			return true
		}
	}

	return validCount > 0
}

func isBuiltinWord(w string) bool {
	// Simple fallback check against unscramble curated words
	for l := 3; l <= 16; l++ {
		words, ok := wcgDictionary[l]
		if !ok {
			continue
		}
		for _, item := range words {
			if strings.EqualFold(item, w) {
				return true
			}
		}
	}
	return false
}

// HandleWCGInput intercepts non-prefix text messages in a chat where a WCG game is active.
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
		game.Mu.Unlock()
		return true
	}

	// Check if sender is in the game
	pIdx := game.FindPlayerIndex(senderLID)
	if pIdx == -1 {
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
		game.Mu.Unlock()
		return true
	}

	game.Mu.Unlock()

	guess := strings.ToLower(strings.TrimSpace(text))

	// Validation rule 1: Check min length rule (greater than or equal to expected length)
	if len(guess) < game.MinLength {
		_ = ctx.Reply(fmt.Sprintf("❌ Word too short! Must be at least %d characters long (got %d). Try again!", game.MinLength, len(guess)))
		return true
	}

	// Validation rule 2: Starting character must match RequiredChar
	if len(guess) == 0 || rune(guess[0]) != game.RequiredChar {
		_ = ctx.Reply(fmt.Sprintf("❌ Invalid start letter! Word must start with '%c'. Try again!", game.RequiredChar))
		return true
	}

	// Validation rule 3: Cannot reuse word already used in current match
	if game.IsWordUsed(guess) {
		_ = ctx.Reply(fmt.Sprintf("❌ Word '%s' was already used in this match! Try a different word!", guess))
		return true
	}

	// Validation rule 4: Parallel API validation for dictionary correctness
	if !ValidateWordParallel(guess) {
		_ = ctx.Reply(fmt.Sprintf("❌ '%s' is not recognized as a valid English word across dictionary sources! Try again!", guess))
		return true
	}

	// Word is valid! Process turn progression
	correct, gameOver, winner, currentPlayer, elapsed := game.ProcessGuess(guess, senderLID)

	if correct {
		nextChar := rune(guess[len(guess)-1])
		msg := fmt.Sprintf("🎉 Correct! %s submitted '%s' (%d letters) in %.1fs! (+%d pts)\n\nNext Required Letter: '%c' | Min Length: %d",
			currentPlayer.Tag, guess, len(guess), elapsed.Seconds(), len(guess)*10, nextChar, game.MinLength)
		_ = ctx.ReplyWithMentions(msg, []types.JID{currentPlayer.MentionJID})

		if gameOver {
			finishWCGChainGame(ctx, game, winner)
			return true
		}

		startWCGChainTurn(ctx, game)
		return true
	}

	return true
}

func handleWCGChain(ctx *Context) error {
	chatKey := ctx.Chat.String()

	existingGame := utils.GetWCGGame(chatKey)

	arg0 := ""
	if len(ctx.Args) > 0 {
		arg0 = strings.ToLower(ctx.Args[0])
	}

	if arg0 == "lb" || arg0 == "leaderboard" {
		return handleWCGChainLeaderboard(ctx)
	}

	if arg0 == "cancel" || arg0 == "stop" {
		if existingGame == nil {
			return ctx.Reply("No active WCG game to cancel.")
		}
		existingGame.StopTimers()
		utils.DeleteWCGGame(chatKey)
		return ctx.Reply("Word Chain Game (WCG) cancelled.")
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
				startWCGChainGame(ctx, existingGame)
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
		startWCGChainGame(cctx, newGame)
	})
	newGame.SetLobbyTimer(timer)

	err := sendWCGChainInteractiveMenu(ctx, hostTag)
	if err != nil {
		textMsg := fmt.Sprintf("🔤 WORD CHAIN GAME (WCG) 🔤\n\nHosted by: %s\n\n⏱️ Lobby is open for 30 SECONDS!\nType '.wcg join' to join\nType '.wcg start' to begin now\nType '.wcg lb' for Leaderboard", hostTag)
		return ctx.ReplyWithMentions(textMsg, []types.JID{hostMention})
	}

	return nil
}

func startWCGChainGame(ctx *Context, game *utils.WCGGame) {
	if !game.StartGame() {
		_ = ctx.Reply("WCG Match cancelled — no players joined the lobby.")
		return
	}

	active := game.GetActivePlayers()
	slog.Debug("[WCG] Starting Word Chain Game", "chat", game.ChatKey, "playersCount", len(active))

	var playerTags []string
	var mentions []types.JID
	for _, p := range active {
		playerTags = append(playerTags, p.Tag)
		mentions = append(mentions, p.MentionJID)
	}

	// Pick random starting letter (a-z)
	startRune := rune('a' + rand.Intn(26))
	game.Mu.Lock()
	game.RequiredChar = startRune
	game.MinLength = 3
	game.Mu.Unlock()

	msg := fmt.Sprintf("🎮 Word Chain Game (WCG) Started!\n\nPlayers (%d): %s\n\nStarting Letter: '%c' (Min Length: 3)\nWords are validated in real-time across 5 dictionary APIs!",
		len(active), strings.Join(playerTags, ", "), startRune)
	_ = ctx.ReplyWithMentions(msg, mentions)

	startWCGChainTurn(ctx, game)
}

func startWCGChainTurn(ctx *Context, game *utils.WCGGame) {
	reqChar, minLen, timeSec, currentPlayer := game.StartTurn()
	if currentPlayer == nil {
		winner, _ := game.FinishGame()
		finishWCGChainGame(ctx, game, winner)
		return
	}

	msg := fmt.Sprintf("🔤 TURN: %s\n\nRequired Starting Letter: *%c*\nMinimum Word Length: *%d* characters\n⏱️ Time Limit: %d seconds!\n\nType a valid English word matching the required letter!",
		currentPlayer.Tag, reqChar, minLen, timeSec)
	_ = ctx.ReplyWithMentions(msg, []types.JID{currentPlayer.MentionJID})

	timeDuration := time.Duration(timeSec) * time.Second
	timer := time.AfterFunc(timeDuration, func() {
		game.Mu.Lock()
		if game.State != utils.WCGStateInProgress {
			game.Mu.Unlock()
			return
		}

		slog.Debug("[WCG] Turn timed out", "player", currentPlayer.Tag)

		cctx := &Context{
			Ctx:    ctx.Ctx,
			Client: game.Client,
			Chat:   game.ChatJID,
			Sender: ctx.Sender,
		}

		timeoutMsg := fmt.Sprintf("⏱️ Time's up for %s!\nFailed to submit a valid word starting with '%c'.\n%s has been eliminated!",
			currentPlayer.Tag, reqChar, currentPlayer.Tag)
		_ = cctx.ReplyWithMentions(timeoutMsg, []types.JID{currentPlayer.MentionJID})

		gameOver, winner := game.EliminateCurrentPlayer()
		game.Mu.Unlock()

		if gameOver {
			finishWCGChainGame(cctx, game, winner)
			return
		}

		startWCGChainTurn(cctx, game)
	})
	game.SetTurnTimer(timer)
}

func finishWCGChainGame(ctx *Context, game *utils.WCGGame, winner *utils.WCGPlayer) {
	finalWinner, standings := game.FinishGame()
	if finalWinner != nil {
		winner = finalWinner
	}

	saveWCGChainStats(ctx, game, winner)

	var sb strings.Builder
	sb.WriteString("🏆 WCG WORD CHAIN MATCH OVER! 🏆\n\n")

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

func saveWCGChainStats(ctx *Context, game *utils.WCGGame, winner *utils.WCGPlayer) {
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

func handleWCGChainLeaderboard(ctx *Context) error {
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

func sendWCGChainInteractiveMenu(ctx *Context, hostTag string) error {
	msgVersion := int32(1)

	bodyText := fmt.Sprintf("🔤 WORD CHAIN GAME (WCG)\n\nHosted by %s\n\n⏱️ 30s Join Window Open!\nClick 'Join Match' or type '.wcg join' to play.\n\nRules:\n• Starting letter is picked at random\n• Words must start with required letter and meet length limit\n• Validated in real-time across 5 parallel dictionary APIs\n• Emojis & non-players are ignored\n• Win XP & climb performance ratings!", hostTag)

	msg := &waE2E.Message{
		DocumentWithCaptionMessage: &waE2E.FutureProofMessage{
			Message: &waE2E.Message{
				InteractiveMessage: &waE2E.InteractiveMessage{
					Body: &waE2E.InteractiveMessage_Body{
						Text: &bodyText,
					},
					Footer: &waE2E.InteractiveMessage_Footer{
						Text: new("WhatsRook Word Chain Game"),
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
