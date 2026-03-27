package tictactoe

import "testing"

func TestApplyMoveValidation(t *testing.T) {
	g := NewGame()

	if _, err := g.ApplyMove(9, 1); err == nil {
		t.Fatalf("expected out-of-range error")
	}
	if _, err := g.ApplyMove(0, 2); err == nil {
		t.Fatalf("expected turn validation error")
	}
	if _, err := g.ApplyMove(0, 1); err != nil {
		t.Fatalf("expected valid first move, got %v", err)
	}
	if _, err := g.ApplyMove(0, 2); err == nil {
		t.Fatalf("expected occupied-cell error")
	}
}

func TestApplyMoveWinner(t *testing.T) {
	g := NewGame()
	moves := []struct {
		cell   int
		symbol int
	}{
		{0, 1}, {3, 2}, {1, 1}, {4, 2}, {2, 1},
	}

	for i, m := range moves {
		res, err := g.ApplyMove(m.cell, m.symbol)
		if err != nil {
			t.Fatalf("move %d failed: %v", i, err)
		}
		if i == len(moves)-1 && res.WinnerSymbol != 1 {
			t.Fatalf("expected winner symbol 1, got %d", res.WinnerSymbol)
		}
	}
}

func TestApplyMoveDraw(t *testing.T) {
	g := NewGame()
	moves := []struct {
		cell   int
		symbol int
	}{
		{0, 1}, {1, 2}, {2, 1},
		{4, 2}, {3, 1}, {5, 2},
		{7, 1}, {6, 2}, {8, 1},
	}

	var draw bool
	for i, m := range moves {
		res, err := g.ApplyMove(m.cell, m.symbol)
		if err != nil {
			t.Fatalf("move %d failed: %v", i, err)
		}
		draw = res.Draw
	}
	if !draw {
		t.Fatalf("expected draw on final move")
	}
}
