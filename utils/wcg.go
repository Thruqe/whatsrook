// Package utils provides the core Word Chain Game (WCG) engine.
package utils

import (
	"math/rand"
	"strings"
	"sync"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

type WCGState int

const (
	WCGStateLobby WCGState = iota
	WCGStateInProgress
)

type WCGPlayer struct {
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

type WCGGame struct {
	Mu             sync.Mutex
	ChatKey        string
	State          WCGState
	HostLID        types.JID
	HostMention    types.JID
	HostTag        string
	Players        []*WCGPlayer
	CurrentTurnIdx int
	RequiredChar   rune
	MinLength      int
	UsedWords      map[string]bool
	TurnStartTime  time.Time
	LobbyTimer     *time.Timer
	TurnTimer      *time.Timer
	GameStartTime  time.Time
	Client         *whatsmeow.Client
	ChatJID        types.JID
}

var (
	wcgMu    sync.Mutex
	wcgGames = make(map[string]*WCGGame) // chat key -> game
)

// IsWCGGameActive returns true if there is an active WCG game (lobby or in-progress) in the chat.
func IsWCGGameActive(chatKey string) bool {
	wcgMu.Lock()
	defer wcgMu.Unlock()
	_, exists := wcgGames[chatKey]
	return exists
}

// GetWCGGame returns the active game for a chat key, or nil.
func GetWCGGame(chatKey string) *WCGGame {
	wcgMu.Lock()
	defer wcgMu.Unlock()
	return wcgGames[chatKey]
}

// CreateWCGGame creates a new lobby for a chat.
func CreateWCGGame(chatKey string, hostLID, hostMention types.JID, hostTag string, chatJID types.JID, client *whatsmeow.Client) *WCGGame {
	game := &WCGGame{
		ChatKey:     chatKey,
		State:       WCGStateLobby,
		HostLID:     hostLID,
		HostMention: hostMention,
		HostTag:     hostTag,
		RequiredChar: rune('a' + rand.Intn(26)),
		MinLength:   3,
		UsedWords:   make(map[string]bool),
		ChatJID:     chatJID,
		Client:      client,
	}

	game.Players = append(game.Players, &WCGPlayer{
		LID:        hostLID,
		MentionJID: hostMention,
		Tag:        hostTag,
		JoinedAt:   time.Now(),
	})

	wcgMu.Lock()
	wcgGames[chatKey] = game
	wcgMu.Unlock()

	return game
}

// DeleteWCGGame removes a game from the active map.
func DeleteWCGGame(chatKey string) {
	wcgMu.Lock()
	defer wcgMu.Unlock()
	delete(wcgGames, chatKey)
}

// AddPlayer adds a player to an existing lobby.
func (g *WCGGame) AddPlayer(lid, mentionJID types.JID, tag string) bool {
	g.Mu.Lock()
	defer g.Mu.Unlock()

	if g.State != WCGStateLobby {
		return false
	}
	if g.FindPlayerIndex(lid) != -1 {
		return false
	}

	g.Players = append(g.Players, &WCGPlayer{
		LID:        lid,
		MentionJID: mentionJID,
		Tag:        tag,
		JoinedAt:   time.Now(),
	})
	return true
}

// FindPlayerIndex returns the index of a player by LID, or -1.
func (g *WCGGame) FindPlayerIndex(lid types.JID) int {
	for i, p := range g.Players {
		if p.LID.User == lid.User {
			return i
		}
	}
	return -1
}

// GetActivePlayers returns all non-eliminated players.
func (g *WCGGame) GetActivePlayers() []*WCGPlayer {
	var active []*WCGPlayer
	for _, p := range g.Players {
		if !p.Eliminated {
			active = append(active, p)
		}
	}
	return active
}

// IsWordUsed checks if a word has already been submitted in the current game.
func (g *WCGGame) IsWordUsed(word string) bool {
	g.Mu.Lock()
	defer g.Mu.Unlock()
	return g.UsedWords[strings.ToLower(word)]
}

// StartGame transitions the game from lobby to in-progress.
func (g *WCGGame) StartGame() bool {
	g.Mu.Lock()
	defer g.Mu.Unlock()

	if g.State == WCGStateInProgress {
		return false
	}

	g.State = WCGStateInProgress
	g.GameStartTime = time.Now()
	g.RequiredChar = rune('a' + rand.Intn(26))
	g.MinLength = 3
	g.CurrentTurnIdx = 0
	g.UsedWords = make(map[string]bool)

	active := g.GetActivePlayers()
	if len(active) == 0 {
		DeleteWCGGame(g.ChatKey)
		return false
	}

	return true
}

// StartTurn sets up turn parameters for current player.
func (g *WCGGame) StartTurn() (reqChar rune, minLen int, timeLimitSec int, currentPlayer *WCGPlayer) {
	g.Mu.Lock()
	defer g.Mu.Unlock()

	active := g.GetActivePlayers()
	if len(active) == 0 {
		return 'a', 3, 0, nil
	}

	if g.CurrentTurnIdx >= len(g.Players) {
		g.CurrentTurnIdx = 0
	}
	for g.Players[g.CurrentTurnIdx].Eliminated {
		g.CurrentTurnIdx = (g.CurrentTurnIdx + 1) % len(g.Players)
	}

	currentPlayer = g.Players[g.CurrentTurnIdx]
	g.TurnStartTime = time.Now()
	timeLimitSec = 25

	return g.RequiredChar, g.MinLength, timeLimitSec, currentPlayer
}

// ProcessGuess processes a valid word submission.
func (g *WCGGame) ProcessGuess(word string, senderLID types.JID) (correct bool, gameOver bool, winner *WCGPlayer, currentPlayer *WCGPlayer, elapsed time.Duration) {
	g.Mu.Lock()
	defer g.Mu.Unlock()

	if g.State != WCGStateInProgress {
		return false, true, nil, nil, 0
	}

	pIdx := g.FindPlayerIndex(senderLID)
	if pIdx == -1 {
		return false, false, nil, nil, 0
	}

	active := g.GetActivePlayers()
	if len(active) == 0 {
		return false, true, nil, nil, 0
	}

	currentPlayer = g.Players[g.CurrentTurnIdx]
	if currentPlayer.LID.User != senderLID.User {
		return false, false, nil, currentPlayer, 0
	}

	word = strings.ToLower(strings.TrimSpace(word))
	elapsed = time.Since(g.TurnStartTime)

	if g.TurnTimer != nil {
		g.TurnTimer.Stop()
	}

	currentPlayer.GuessesCount++
	currentPlayer.TotalTimeMs += elapsed.Milliseconds()

	// Record used word & score
	g.UsedWords[word] = true
	currentPlayer.Score += len(word) * 10
	currentPlayer.CorrectGuesses++

	// Next required starting character is the last character of the submitted word
	g.RequiredChar = rune(word[len(word)-1])

	// Slowly increase minimum length requirement as word chain grows
	if len(g.UsedWords)%3 == 0 && g.MinLength < 10 {
		g.MinLength++
	}

	g.advanceTurnUnsafe()
	return true, false, nil, currentPlayer, elapsed
}

func (g *WCGGame) advanceTurnUnsafe() {
	g.CurrentTurnIdx = (g.CurrentTurnIdx + 1) % len(g.Players)
	for g.Players[g.CurrentTurnIdx].Eliminated {
		g.CurrentTurnIdx = (g.CurrentTurnIdx + 1) % len(g.Players)
	}
}

// EliminateCurrentPlayer eliminates the current turn player and advances.
func (g *WCGGame) EliminateCurrentPlayer() (gameOver bool, winner *WCGPlayer) {
	g.Mu.Lock()
	defer g.Mu.Unlock()

	if g.CurrentTurnIdx < len(g.Players) {
		g.Players[g.CurrentTurnIdx].Eliminated = true
	}

	rem := g.GetActivePlayers()
	if len(rem) <= 1 {
		if len(rem) == 1 {
			winner = rem[0]
		}
		return true, winner
	}

	g.advanceTurnUnsafe()
	return false, nil
}

// FinishGame cleans up the game and returns final standings.
func (g *WCGGame) FinishGame() (winner *WCGPlayer, standings []*WCGPlayer) {
	g.Mu.Lock()
	defer g.Mu.Unlock()

	if g.TurnTimer != nil {
		g.TurnTimer.Stop()
	}

	active := g.GetActivePlayers()
	if len(active) == 1 {
		winner = active[0]
	}

	standings = make([]*WCGPlayer, len(g.Players))
	copy(standings, g.Players)
	for i := 0; i < len(standings); i++ {
		for j := i + 1; j < len(standings); j++ {
			if standings[j].Score > standings[i].Score {
				standings[i], standings[j] = standings[j], standings[i]
			}
		}
	}

	DeleteWCGGame(g.ChatKey)
	return winner, standings
}

// GetSortedPlayers returns players sorted by score descending.
func (g *WCGGame) GetSortedPlayers() []*WCGPlayer {
	g.Mu.Lock()
	defer g.Mu.Unlock()

	sorted := make([]*WCGPlayer, len(g.Players))
	copy(sorted, g.Players)
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j].Score > sorted[i].Score {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}
	return sorted
}

func (g *WCGGame) SetLobbyTimer(timer *time.Timer) {
	g.Mu.Lock()
	defer g.Mu.Unlock()
	g.LobbyTimer = timer
}

func (g *WCGGame) SetTurnTimer(timer *time.Timer) {
	g.Mu.Lock()
	defer g.Mu.Unlock()
	g.TurnTimer = timer
}

func (g *WCGGame) StopTimers() {
	g.Mu.Lock()
	defer g.Mu.Unlock()
	if g.LobbyTimer != nil {
		g.LobbyTimer.Stop()
	}
	if g.TurnTimer != nil {
		g.TurnTimer.Stop()
	}
}
