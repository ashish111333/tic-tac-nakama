package match

import (
	"fmt"
	"time"

	"tic-tac-nakama/internal/domain/tictactoe"
)

const DefaultTurnLimit = 30

type Player struct {
	UserID   string
	Username string
}

type State struct {
	MatchID      string
	Mode         tictactoe.Mode
	TurnLimitSec int
	Game         tictactoe.Game
	Players      map[string]Player
	Symbols      map[string]int
	TurnUserID   string
	Status       tictactoe.Status
	WinnerUserID string
	MoveDeadline time.Time
	LastError    string
	Ended        bool
	EmptyTicks   int
}

type Snapshot struct {
	MatchID      string         `json:"match_id"`
	Mode         tictactoe.Mode `json:"mode"`
	Status       string         `json:"status"`
	Board        [9]int         `json:"board"`
	TurnUserID   string         `json:"turn_user_id,omitempty"`
	WinnerUserID string         `json:"winner_user_id,omitempty"`
	MoveDeadline int64          `json:"move_deadline_unix,omitempty"`
	Players      map[string]int `json:"players"`
	LastError    string         `json:"last_error,omitempty"`
}

func NewState(matchID string, mode tictactoe.Mode, turnLimitSec int) *State {
	if turnLimitSec <= 0 {
		turnLimitSec = DefaultTurnLimit
	}
	return &State{
		MatchID:      matchID,
		Mode:         mode,
		TurnLimitSec: turnLimitSec,
		Game:         tictactoe.NewGame(),
		Players:      make(map[string]Player),
		Symbols:      make(map[string]int),
		Status:       tictactoe.StatusWaiting,
	}
}

func (s *State) JoinAttempt(userID string) error {
	if s.Ended {
		return fmt.Errorf("match is finished")
	}
	if _, exists := s.Players[userID]; exists {
		return nil
	}
	if len(s.Players) >= 2 {
		return fmt.Errorf("room is full")
	}
	return nil
}

func (s *State) Join(joiners []Player, now time.Time) {
	for _, p := range joiners {
		s.Players[p.UserID] = p
		if _, ok := s.Symbols[p.UserID]; !ok {
			if len(s.Symbols) == 0 {
				s.Symbols[p.UserID] = 1
			} else {
				s.Symbols[p.UserID] = 2
			}
		}
	}
	if len(s.Players) == 2 && s.Status == tictactoe.StatusWaiting {
		s.Status = tictactoe.StatusInProgress
		s.TurnUserID = s.userBySymbol(1)
		s.MoveDeadline = now.UTC().Add(time.Duration(s.TurnLimitSec) * time.Second)
	}
}

func (s *State) Leave(userIDs []string) {
	for _, uid := range userIDs {
		delete(s.Players, uid)
	}

	if s.Status == tictactoe.StatusInProgress && len(s.Players) == 1 {
		for uid := range s.Players {
			s.WinnerUserID = uid
			break
		}
		s.Status = tictactoe.StatusFinished
		s.Ended = true
		s.LastError = "opponent disconnected"
	}
	if len(s.Players) == 0 {
		s.EmptyTicks = 1
	}
}

func (s *State) HandleMove(userID string, cell int, now time.Time) (finished bool, err error) {
	if s.Status != tictactoe.StatusInProgress {
		s.LastError = "match is not in progress"
		return false, fmt.Errorf(s.LastError)
	}
	if userID != s.TurnUserID {
		s.LastError = "not your turn"
		return false, fmt.Errorf(s.LastError)
	}

	symbol := s.Symbols[userID]
	result, moveErr := s.Game.ApplyMove(cell, symbol)
	if moveErr != nil {
		s.LastError = moveErr.Error()
		return false, moveErr
	}

	s.LastError = ""
	if result.WinnerSymbol != 0 {
		s.WinnerUserID = s.userBySymbol(result.WinnerSymbol)
		s.Status = tictactoe.StatusFinished
		s.Ended = true
		return true, nil
	}
	if result.Draw {
		s.Status = tictactoe.StatusFinished
		s.Ended = true
		return true, nil
	}

	s.TurnUserID = s.userBySymbol(result.NextTurn)
	if s.Mode == tictactoe.ModeTimed {
		s.MoveDeadline = now.UTC().Add(time.Duration(s.TurnLimitSec) * time.Second)
	}
	return false, nil
}

func (s *State) Tick(now time.Time) bool {
	if s.Status == tictactoe.StatusInProgress && s.Mode == tictactoe.ModeTimed && !s.MoveDeadline.IsZero() && now.UTC().After(s.MoveDeadline) {
		s.WinnerUserID = s.otherUser(s.TurnUserID)
		s.Status = tictactoe.StatusFinished
		s.Ended = true
		s.LastError = "turn timed out"
		return true
	}
	return false
}

func (s *State) Snapshot() Snapshot {
	out := Snapshot{
		MatchID:      s.MatchID,
		Mode:         s.Mode,
		Status:       string(s.Status),
		Board:        s.Game.Board,
		TurnUserID:   s.TurnUserID,
		WinnerUserID: s.WinnerUserID,
		Players:      s.Symbols,
		LastError:    s.LastError,
	}
	if s.Mode == tictactoe.ModeTimed && s.Status == tictactoe.StatusInProgress {
		out.MoveDeadline = s.MoveDeadline.Unix()
	}
	return out
}

func (s *State) userBySymbol(symbol int) string {
	for uid, sym := range s.Symbols {
		if sym == symbol {
			return uid
		}
	}
	return ""
}

func (s *State) otherUser(current string) string {
	for uid := range s.Symbols {
		if uid != current {
			return uid
		}
	}
	return ""
}
