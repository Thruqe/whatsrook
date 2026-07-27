package commands

import (
	"testing"

	"go.mau.fi/whatsmeow/types"
)

func TestTicTacToeRegistration(t *testing.T) {
	cmd, ok := Get("tictactoe")
	if !ok {
		t.Fatal("expected 'tictactoe' command to be registered")
	}

	if cmd.Category != "games" {
		t.Errorf("expected category 'games', got %q", cmd.Category)
	}

	if !cmd.IsPublic {
		t.Error("expected tictactoe command to be public")
	}

	foundAlias := false
	for _, a := range cmd.Aliases {
		if a == "ttt" {
			foundAlias = true
			break
		}
	}
	if !foundAlias {
		t.Error("expected alias 'ttt' to be registered")
	}
}

func TestLeaderboardRegistration(t *testing.T) {
	cmd, ok := Get("leaderboard")
	if !ok {
		t.Fatal("expected 'leaderboard' command to be registered")
	}

	if cmd.Category != "games" {
		t.Errorf("expected category 'games', got %q", cmd.Category)
	}

	if !cmd.IsPublic {
		t.Error("expected leaderboard command to be public")
	}
}

func TestTTTBoardRendering(t *testing.T) {
	var board [9]string
	grid := renderTTTGrid(&board)
	expected := " 1 | 2 | 3 \n---+---+---\n 4 | 5 | 6 \n---+---+---\n 7 | 8 | 9 "
	if grid != expected {
		t.Errorf("expected grid:\n%s\ngot:\n%s", expected, grid)
	}
}

func TestTTTWinnerCheck(t *testing.T) {
	var board [9]string
	if checkTTTWinner(&board) != "" {
		t.Error("expected no winner for empty board")
	}

	board[0], board[1], board[2] = "X", "X", "X"
	if checkTTTWinner(&board) != "X" {
		t.Error("expected X to win on top row")
	}

	var board2 [9]string
	board2[0], board2[4], board2[8] = "O", "O", "O"
	if checkTTTWinner(&board2) != "O" {
		t.Error("expected O to win on diagonal")
	}
}

func TestTTTFullCheck(t *testing.T) {
	var board [9]string
	if isTTTFull(&board) {
		t.Error("expected empty board not to be full")
	}

	for i := 0; i < 9; i++ {
		board[i] = "X"
	}
	if !isTTTFull(&board) {
		t.Error("expected full board to return true")
	}
}

func TestMinimaxBotUnbeatable(t *testing.T) {
	// If X plays top-left (0), Minimax AI (O) must pick center (4) to prevent advantage
	var board [9]string
	board[0] = "X"

	move := bestTTTMove(&board)
	if move != 4 {
		t.Errorf("expected bot to choose center cell 4, got %d", move)
	}

	// If X threatens winning line (0, 1), bot must block at 2
	var board2 [9]string
	board2[0] = "X"
	board2[4] = "O"
	board2[1] = "X"

	move2 := bestTTTMove(&board2)
	if move2 != 2 {
		t.Errorf("expected bot to block winning move at cell 2, got %d", move2)
	}
}

func TestTTTGameLogic(t *testing.T) {
	userX, _ := types.ParseJID("1111111111@s.whatsapp.net")
	userO, _ := types.ParseJID("2222222222@s.whatsapp.net")

	game := &tttGame{
		PlayerX: userX,
		PlayerO: userO,
		Turn:    userX,
	}

	if getSymbol(game, userX) != "X" || getSymbol(game, userO) != "O" {
		t.Error("symbol matching failed")
	}
}
