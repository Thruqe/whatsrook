// Tic-Tac-Toe command – interactive turn-based Tic-Tac-Toe game with AI opponent, random starter, and XP leaderboard.
package commands

import (
	"fmt"
	"log/slog"
	"math"
	"math/rand"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"whatsrook/store/sqlstore"
	"whatsrook/utils"

	"go.mau.fi/whatsmeow/types"
)

var botJID = types.NewJID("whatsrook_bot", "s.whatsapp.net")

func init() {
	Register(&Command{
		Name:        "tictactoe",
		Aliases:     []string{"ttt"},
		Description: "Play Tic-Tac-Toe against the bot AI or another user",
		Category:    "games",
		IsPublic:    true,
		Handler:     handleTicTacToe,
	})

	Register(&Command{
		Name:        "leaderboard",
		Aliases:     []string{"lb", "top", "xp"},
		Description: "Show overall XP & Tic-Tac-Toe leaderboard",
		Category:    "games",
		IsPublic:    true,
		Handler:     handleLeaderboard,
	})
}

type tttGame struct {
	Board          [9]string // "", "X", "O"
	PlayerX        types.JID // raw LID for turn tracking
	PlayerO        types.JID // raw LID or botJID for turn tracking
	PlayerXMention types.JID // resolved phone JID for mentions
	PlayerOMention types.JID // resolved phone JID for mentions
	Turn           types.JID // raw LID of whose turn it is
	PlayerXTag     string
	PlayerOTag     string
	IsBotGame      bool
}

var (
	tttMu    sync.Mutex
	tttGames = make(map[string]*tttGame) // chat JID string -> game
	rng      = rand.New(rand.NewSource(time.Now().UnixNano()))
)

// IsTTTGameActive returns true if there is an ongoing Tic-Tac-Toe game in the given chat JID.
func IsTTTGameActive(chatJID string) bool {
	tttMu.Lock()
	defer tttMu.Unlock()
	_, exists := tttGames[chatJID]
	return exists
}

func handleTicTacToe(ctx *Context) error {
	tttMu.Lock()
	defer tttMu.Unlock()

	chatKey := ctx.Chat.String()
	game, exists := tttGames[chatKey]

	if len(ctx.Args) == 0 {
		if !exists {
			return ctx.Reply("No Tic-Tac-Toe game active in this chat.\nStart a game against AI:\n.ttt bot\nOr play against a friend:\n.ttt @user")
		}
		return ctx.Reply(renderTTTBoard(game))
	}

	arg0 := strings.ToLower(ctx.Args[0])

	if arg0 == "cancel" || arg0 == "reset" || arg0 == "stop" {
		if !exists {
			return ctx.Reply("No active Tic-Tac-Toe game to cancel.")
		}
		delete(tttGames, chatKey)
		return ctx.Reply("Tic-Tac-Toe game cancelled.")
	}

	if !exists {
		var playerO types.JID
		var oTag string
		isBotGame := false

		// rawSenderLID is used for turn tracking — always LID format matching incoming senders.
		rawSenderLID := ctx.Sender.ToNonAD()
		userMentionJID, username := ctx.ResolveMention(rawSenderLID)
		userTag := "@" + username

		var playerOMention types.JID
		if arg0 == "bot" || arg0 == "ai" || arg0 == "me" || arg0 == "solo" {
			// Use the bot's own JID so WhatsApp renders it as a real interactive mention.
			rawBot := ctx.Client.Store.ID.ToNonAD()
			playerO = rawBot
			playerOMention, _ = ctx.ResolveMention(rawBot)
			oTag, _ = ctx.FormatMention(rawBot)
			isBotGame = true
		} else if len(ctx.Evt.Message.GetExtendedTextMessage().GetContextInfo().GetMentionedJID()) > 0 {
			mentionedRaw := ctx.Evt.Message.GetExtendedTextMessage().GetContextInfo().GetMentionedJID()[0]
			parsedJID, err := types.ParseJID(mentionedRaw)
			if err != nil {
				return ctx.Reply("Invalid user mention for opponent.")
			}
			// Store raw LID for turn tracking, resolved phone JID for mentions.
			playerO = parsedJID.ToNonAD()
			playerOMention, _ = ctx.ResolveMention(playerO)
			oTag, _ = ctx.FormatMention(playerO)
		} else {
			return ctx.Reply("To start a Tic-Tac-Toe game, play against AI:\n.ttt bot\nOr tag an opponent:\n.ttt @user")
		}

		// Randomly decide who starts first (bot games only)
		botStarts := isBotGame && rng.Intn(2) == 0
		firstTurn := rawSenderLID
		firstTag := userTag
		if botStarts {
			firstTurn = botJID
			firstTag = oTag
		}

		newGame := &tttGame{
			PlayerX:        rawSenderLID,   // LID for turn tracking
			PlayerO:        playerO,        // LID or botJID for turn tracking
			PlayerXMention: userMentionJID, // phone JID for mentions
			PlayerOMention: playerOMention, // phone JID for mentions
			Turn:           firstTurn,
			PlayerXTag:     userTag,
			PlayerOTag:     oTag,
			IsBotGame:      isBotGame,
		}

		// If Bot starts first, compute its first move immediately
		botFirstMsg := ""
		if botStarts {
			botMove := bestTTTMove(&newGame.Board)
			if botMove != -1 {
				newGame.Board[botMove] = "O"
			}
			newGame.Turn = rawSenderLID
			botFirstMsg = fmt.Sprintf("\n\nAI decided to go first and placed move at position %d!", botMove+1)
		}

		slog.Debug("[TTT] Creating new game", "chat", chatKey, "rawSenderLID", rawSenderLID.String(), "mentionJID", userMentionJID.String(), "botStarts", botStarts, "firstTurn", firstTurn.String())

		tttGames[chatKey] = newGame

		msg := fmt.Sprintf("Tic-Tac-Toe Started!\n\nPlayer X: %s\nPlayer O: %s\n\nTurn: %s (X)%s\n\n%s\n\nMake a move by sending a number 1-9",
			userTag, oTag, firstTag, botFirstMsg, renderTTTGrid(&newGame.Board))

		mentions := []types.JID{userMentionJID, playerOMention}
		return ctx.ReplyWithMentions(msg, mentions)
	}

	pos, err := strconv.Atoi(arg0)
	if err != nil || pos < 1 || pos > 9 {
		return ctx.Reply("Invalid move. Enter a position from 1 to 9, or '.ttt cancel' to reset game.")
	}

	idx := pos - 1
	if game.Board[idx] != "" {
		return ctx.Reply("Position already taken. Choose an empty spot (1-9).")
	}

	// Incoming sender is always LID format; game.PlayerX/Turn are also stored as LID.
	senderLID := ctx.Sender.ToNonAD()
	slog.Debug("[TTT] Processing move", "chat", chatKey, "senderLID", senderLID.String(), "senderUser", senderLID.User, "gameTurnUser", game.Turn.User, "gameTurnJID", game.Turn.String(), "playerXUser", game.PlayerX.User, "isBotGame", game.IsBotGame)

	if senderLID.User != game.Turn.User {
		slog.Warn("[TTT] Move rejected: not sender's turn", "senderLID", senderLID.String(), "senderUser", senderLID.User, "expectedTurnUser", game.Turn.User)
		return ctx.Reply("It is not your turn.")
	}

	// 1. Determine symbol for this sender
	symbol := "X"
	if senderLID.User == game.PlayerO.User && !game.IsBotGame {
		symbol = "O"
	}

	game.Board[idx] = symbol

	// Check if user won or drew
	if winner := checkTTTWinner(&game.Board); winner != "" {
		delete(tttGames, chatKey)
		winnerTag := game.PlayerXTag
		if winner == "O" {
			winnerTag = game.PlayerOTag
		}

		if game.IsBotGame {
			if winner == "X" {
				awardTTTXP(ctx, game.PlayerXMention, 50, "win")
			} else {
				awardTTTXP(ctx, game.PlayerXMention, 10, "loss")
			}
		} else {
			awardTTTXP(ctx, game.PlayerXMention, 50, "win")
			awardTTTXP(ctx, game.PlayerOMention, 10, "loss")
		}

		var winnerMentionJID types.JID
		if winner == "O" {
			winnerMentionJID = game.PlayerOMention
		} else {
			winnerMentionJID = game.PlayerXMention
		}
		msg := fmt.Sprintf("Game Over!\n\nWinner: %s (%s)\n+50 XP awarded!\n\n%s", winnerTag, winner, renderTTTGrid(&game.Board))
		return ctx.ReplyWithMentions(msg, []types.JID{winnerMentionJID})
	}

	if isTTTFull(&game.Board) {
		delete(tttGames, chatKey)
		awardTTTXP(ctx, game.PlayerXMention, 20, "draw")
		if !game.IsBotGame {
			awardTTTXP(ctx, game.PlayerOMention, 20, "draw")
		}
		msg := fmt.Sprintf("Game Over! It's a draw!\n+20 XP awarded to both players!\n\n%s", renderTTTGrid(&game.Board))
		return ctx.Reply(msg)
	}

	// 2. If playing against Bot, Bot computes smart move (Minimax AI)
	if game.IsBotGame {
		botMove := bestTTTMove(&game.Board)
		if botMove != -1 {
			game.Board[botMove] = "O"
		}

		if winner := checkTTTWinner(&game.Board); winner != "" {
			delete(tttGames, chatKey)
			awardTTTXP(ctx, game.PlayerXMention, 10, "loss")
			msg := fmt.Sprintf("Game Over!\n\nWinner: %s (O)\nBetter luck next time (+10 XP)!\n\n%s", game.PlayerOTag, renderTTTGrid(&game.Board))
			return ctx.ReplyWithMentions(msg, []types.JID{game.PlayerXMention, game.PlayerOMention})
		}

		if isTTTFull(&game.Board) {
			delete(tttGames, chatKey)
			awardTTTXP(ctx, game.PlayerX, 20, "draw")
			msg := fmt.Sprintf("Game Over! It's a draw!\n+20 XP awarded!\n\n%s", renderTTTGrid(&game.Board))
			return ctx.Reply(msg)
		}

		game.Turn = game.PlayerX // PlayerX is the raw LID — correct for turn tracking
		msg := fmt.Sprintf("Move placed!\n\nAI played position %d.\nTurn: %s (X)\n\n%s\n\nSend 1-9 to make your next move",
			botMove+1, game.PlayerXTag, renderTTTGrid(&game.Board))
		return ctx.ReplyWithMentions(msg, []types.JID{game.PlayerXMention, game.PlayerOMention})
	}

	// 2P game turn switch (both stored as LID)
	nextTurn := game.PlayerO
	nextTag := game.PlayerOTag
	nextMention := game.PlayerOMention
	if senderLID.User == game.PlayerO.User {
		nextTurn = game.PlayerX
		nextTag = game.PlayerXTag
		nextMention = game.PlayerXMention
	}
	game.Turn = nextTurn

	msg := fmt.Sprintf("Move placed!\n\nTurn: %s (%s)\n\n%s\n\nSend 1-9 to make your move",
		nextTag, getSymbol(game, nextTurn), renderTTTGrid(&game.Board))
	return ctx.ReplyWithMentions(msg, []types.JID{nextMention})
}

func awardTTTXP(ctx *Context, userJID types.JID, amount int, resultType string) {
	// Scores in DM games are not added to group leaderboards
	if ctx.Chat.Server != "g.us" {
		return
	}

	s, ok := ctx.Client.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return
	}
	db := s.GetDB()
	if db == nil {
		return
	}

	winInc, lossInc, drawInc := 0, 0, 0
	switch resultType {
	case "win":
		winInc = 1
	case "loss":
		lossInc = 1
	case "draw":
		drawInc = 1
	}

	groupJID := ctx.Chat.ToNonAD().String()
	normJID := NormalizeUserJID(ctx.Ctx, ctx.Client, userJID)
	cleanJID := normJID.String()

	_, _ = db.Exec(ctx.Ctx, `INSERT INTO bot_group_user_xp (group_jid, user_jid, xp, ttt_wins, ttt_losses, ttt_draws)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT(group_jid, user_jid) DO UPDATE SET
			xp = MAX(0, bot_group_user_xp.xp + EXCLUDED.xp),
			ttt_wins = bot_group_user_xp.ttt_wins + EXCLUDED.ttt_wins,
			ttt_losses = bot_group_user_xp.ttt_losses + EXCLUDED.ttt_losses,
			ttt_draws = bot_group_user_xp.ttt_draws + EXCLUDED.ttt_draws`,
		groupJID, cleanJID, amount, winInc, lossInc, drawInc)
}

func handleLeaderboard(ctx *Context) error {
	if ctx.Chat.Server != "g.us" {
		return ctx.Reply("Leaderboards are group-specific! Please use .leaderboard inside a group chat to view that group's leaderboard.")
	}

	s, ok := ctx.Client.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return ctx.Reply("Leaderboard store unavailable.")
	}
	db := s.GetDB()
	if db == nil {
		return ctx.Reply("Database connection unavailable.")
	}

	groupJID := ctx.Chat.ToNonAD().String()

	// Fetch group name
	groupName := "Group"
	if info, err := ctx.Client.GetGroupInfo(ctx.Ctx, ctx.Chat); err == nil && info != nil {
		if info.GroupName.Name != "" {
			groupName = info.GroupName.Name
		} else if info.Name != "" {
			groupName = info.Name
		}
	}

	rows, err := db.Query(ctx.Ctx, `SELECT user_jid, xp, ttt_wins, ttt_losses, ttt_draws, COALESCE(wcg_wins, 0), COALESCE(wcg_games, 0), COALESCE(wcg_rating, 1000) 
		FROM bot_group_user_xp 
		WHERE group_jid = $1`, groupJID)
	if err != nil {
		return ctx.Reply("Failed to fetch group leaderboard.")
	}
	defer rows.Close()

	type lbEntry struct {
		jid       types.JID
		tag       string
		xp        int
		title     string
		tttWins   int
		tttLosses int
		tttDraws  int
		wcgWins   int
		wcgGames  int
		rating    int
	}

	mergedMap := make(map[string]*lbEntry)
	var mapKeys []string

	for rows.Next() {
		var jidStr string
		var xp, tWins, tLosses, tDraws, wWins, wGames, rating int
		if err := rows.Scan(&jidStr, &xp, &tWins, &tLosses, &tDraws, &wWins, &wGames, &rating); err == nil {
			if rating == 0 {
				rating = 1000
			}
			parsed, pErr := types.ParseJID(jidStr)
			if pErr != nil {
				continue
			}
			normJID := NormalizeUserJID(ctx.Ctx, ctx.Client, parsed)
			key := normJID.String()

			existing, found := mergedMap[key]
			if !found {
				tag, resolved := ctx.FormatMention(normJID)
				entry := &lbEntry{
					jid:       resolved,
					tag:       tag,
					xp:        xp,
					tttWins:   tWins,
					tttLosses: tLosses,
					tttDraws:  tDraws,
					wcgWins:   wWins,
					wcgGames:  wGames,
					rating:    rating,
				}
				mergedMap[key] = entry
				mapKeys = append(mapKeys, key)
			} else {
				existing.xp += xp
				existing.tttWins += tWins
				existing.tttLosses += tLosses
				existing.tttDraws += tDraws
				existing.wcgWins += wWins
				existing.wcgGames += wGames
				if rating > existing.rating {
					existing.rating = rating
				}
			}
		}
	}

	var entries []lbEntry
	for _, k := range mapKeys {
		e := mergedMap[k]
		e.title = utils.GetCXPTitle(e.xp)
		entries = append(entries, *e)
	}

	slices.SortFunc(entries, func(a, b lbEntry) int {
		return b.xp - a.xp
	})

	if len(entries) > 10 {
		entries = entries[:10]
	}

	if len(entries) == 0 {
		return ctx.Reply(fmt.Sprintf("%s Leaderboard is currently empty! Play games in this group to earn points and rank up.", groupName))
	}

	var mentions []types.JID
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s Leaderboard\n\n", groupName)

	for i, e := range entries {
		fmt.Fprintf(&sb, "%d. %s — %s (%d CXP)\n   Rating: %d | TTT: %dW/%dL/%dD | WCG: %dW/%dG\n\n",
			i+1, e.tag, e.title, e.xp, e.rating, e.tttWins, e.tttLosses, e.tttDraws, e.wcgWins, e.wcgGames)
		mentions = append(mentions, e.jid)
	}

	return ctx.ReplyWithMentions(strings.TrimSpace(sb.String()), mentions)
}

func getSymbol(g *tttGame, p types.JID) string {
	if p.User == g.PlayerX.User {
		return "X"
	}
	return "O"
}

func renderTTTBoard(g *tttGame) string {
	// Turn is stored as LID; compare against PlayerX (also LID) or bot JID.
	turnTag := g.PlayerXTag
	if g.Turn.User == g.PlayerO.User || g.Turn.User == botJID.User {
		turnTag = g.PlayerOTag
	}
	return fmt.Sprintf("Tic-Tac-Toe Game\n\nPlayer X: %s\nPlayer O: %s\nTurn: %s\n\n%s",
		g.PlayerXTag, g.PlayerOTag, turnTag, renderTTTGrid(&g.Board))
}

func renderTTTGrid(board *[9]string) string {
	display := make([]string, 9)
	for i := 0; i < 9; i++ {
		if board[i] == "" {
			display[i] = strconv.Itoa(i + 1)
		} else {
			display[i] = board[i]
		}
	}
	return fmt.Sprintf(
		" %s | %s | %s \n"+
			"---+---+---\n"+
			" %s | %s | %s \n"+
			"---+---+---\n"+
			" %s | %s | %s ",
		display[0], display[1], display[2],
		display[3], display[4], display[5],
		display[6], display[7], display[8],
	)
}

func checkTTTWinner(b *[9]string) string {
	lines := [][3]int{
		{0, 1, 2}, {3, 4, 5}, {6, 7, 8}, // rows
		{0, 3, 6}, {1, 4, 7}, {2, 5, 8}, // cols
		{0, 4, 8}, {2, 4, 6}, // diagonals
	}
	for _, l := range lines {
		if b[l[0]] != "" && b[l[0]] == b[l[1]] && b[l[1]] == b[l[2]] {
			return b[l[0]]
		}
	}
	return ""
}

func isTTTFull(b *[9]string) bool {
	for _, cell := range b {
		if cell == "" {
			return false
		}
	}
	return true
}

// Minimax algorithm for unbeatable AI bot ("O")
func bestTTTMove(board *[9]string) int {
	bestScore := math.MinInt32
	move := -1

	for i := 0; i < 9; i++ {
		if board[i] == "" {
			board[i] = "O"
			score := minimax(board, 0, false)
			board[i] = ""
			if score > bestScore {
				bestScore = score
				move = i
			}
		}
	}
	return move
}

func minimax(board *[9]string, depth int, isMaximizing bool) int {
	winner := checkTTTWinner(board)
	if winner == "O" {
		return 10 - depth
	}
	if winner == "X" {
		return depth - 10
	}
	if isTTTFull(board) {
		return 0
	}

	if isMaximizing {
		bestScore := math.MinInt32
		for i := 0; i < 9; i++ {
			if board[i] == "" {
				board[i] = "O"
				score := minimax(board, depth+1, false)
				board[i] = ""
				if score > bestScore {
					bestScore = score
				}
			}
		}
		return bestScore
	} else {
		bestScore := math.MaxInt32
		for i := 0; i < 9; i++ {
			if board[i] == "" {
				board[i] = "X"
				score := minimax(board, depth+1, true)
				board[i] = ""
				if score < bestScore {
					bestScore = score
				}
			}
		}
		return bestScore
	}
}
