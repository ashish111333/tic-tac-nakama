package rooms

import (
	"sort"
	"sync"
	"time"

	"tic-tac-nakama/internal/domain/tictactoe"
)

type Meta struct {
	MatchID   string         `json:"match_id"`
	Mode      tictactoe.Mode `json:"mode"`
	Open      bool           `json:"open"`
	Players   int            `json:"players"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

type Registry struct {
	mu    sync.RWMutex
	rooms map[string]Meta
}

func NewRegistry() *Registry {
	return &Registry{rooms: make(map[string]Meta)}
}

func (r *Registry) Upsert(meta Meta) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rooms[meta.MatchID] = meta
}

func (r *Registry) Get(id string) (Meta, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	m, ok := r.rooms[id]
	return m, ok
}

func (r *Registry) Delete(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.rooms, id)
}

func (r *Registry) List(mode tictactoe.Mode, onlyOpen bool, limit int) []Meta {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]Meta, 0, len(r.rooms))
	for _, rm := range r.rooms {
		if mode != "" && rm.Mode != mode {
			continue
		}
		if onlyOpen && !rm.Open {
			continue
		}
		out = append(out, rm)
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})

	if limit > 0 && len(out) > limit {
		return out[:limit]
	}
	return out
}
