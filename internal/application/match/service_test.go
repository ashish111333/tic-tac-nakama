package match

import (
	"testing"
	"time"

	"tic-tac-nakama/internal/domain/tictactoe"
)

func TestJoinAndStartFlow(t *testing.T) {
	now := time.Now().UTC()
	s := NewState("m1", tictactoe.ModeClassic, 30)

	s.Join([]Player{{UserID: "u1", Username: "a"}}, now)
	if s.Status != tictactoe.StatusWaiting {
		t.Fatalf("expected waiting after first join")
	}

	s.Join([]Player{{UserID: "u2", Username: "b"}}, now)
	if s.Status != tictactoe.StatusInProgress {
		t.Fatalf("expected in_progress after second join")
	}
	if s.TurnUserID == "" {
		t.Fatalf("expected turn user id to be set")
	}
}

func TestHandleMoveAndWin(t *testing.T) {
	now := time.Now().UTC()
	s := NewState("m1", tictactoe.ModeClassic, 30)
	s.Join([]Player{{UserID: "u1", Username: "a"}, {UserID: "u2", Username: "b"}}, now)

	sequence := []struct {
		uid  string
		cell int
	}{
		{s.TurnUserID, 0},
		{s.otherUser(s.TurnUserID), 3},
		{s.TurnUserID, 1},
		{s.otherUser(s.TurnUserID), 4},
		{s.TurnUserID, 2},
	}

	for i, mv := range sequence {
		finished, err := s.HandleMove(mv.uid, mv.cell, now)
		if err != nil {
			t.Fatalf("move %d failed: %v", i, err)
		}
		if i < len(sequence)-1 && finished {
			t.Fatalf("unexpected early finish")
		}
	}
	if s.Status != tictactoe.StatusFinished || s.WinnerUserID == "" {
		t.Fatalf("expected finished game with winner")
	}
}

func TestTimedModeTimeout(t *testing.T) {
	now := time.Now().UTC()
	s := NewState("m1", tictactoe.ModeTimed, 1)
	s.Join([]Player{{UserID: "u1", Username: "a"}, {UserID: "u2", Username: "b"}}, now)

	triggered := s.Tick(now.Add(2 * time.Second))
	if !triggered {
		t.Fatalf("expected timeout trigger")
	}
	if s.Status != tictactoe.StatusFinished || s.WinnerUserID == "" {
		t.Fatalf("expected timeout to finish match with winner")
	}
}
