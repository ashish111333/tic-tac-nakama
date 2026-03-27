package rooms

import (
	"testing"
	"time"

	"tic-tac-nakama/internal/domain/tictactoe"
)

func TestRegistryFiltersAndLimit(t *testing.T) {
	r := NewRegistry()
	now := time.Now().UTC()
	r.Upsert(Meta{MatchID: "a", Mode: tictactoe.ModeClassic, Open: true, CreatedAt: now.Add(-2 * time.Minute)})
	r.Upsert(Meta{MatchID: "b", Mode: tictactoe.ModeTimed, Open: true, CreatedAt: now.Add(-1 * time.Minute)})
	r.Upsert(Meta{MatchID: "c", Mode: tictactoe.ModeClassic, Open: false, CreatedAt: now})

	openClassic := r.List(tictactoe.ModeClassic, true, 10)
	if len(openClassic) != 1 || openClassic[0].MatchID != "a" {
		t.Fatalf("unexpected filtered result: %+v", openClassic)
	}

	limited := r.List("", false, 2)
	if len(limited) != 2 {
		t.Fatalf("expected 2 results, got %d", len(limited))
	}
	if limited[0].MatchID != "a" || limited[1].MatchID != "b" {
		t.Fatalf("expected chronological ordering by CreatedAt")
	}
}
