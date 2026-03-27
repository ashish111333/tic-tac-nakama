package nakama

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	appmatch "tic-tac-nakama/internal/application/match"
	"tic-tac-nakama/internal/domain/rooms"
	"tic-tac-nakama/internal/domain/tictactoe"

	"github.com/heroiclabs/nakama-common/runtime"
)

const (
	matchName     = "tic_tac_toe_match"
	leaderboardID = "ttt_global"
	tickRate      = 5
	moveOpCode    = 1
)

type playerStats struct {
	Wins       int `json:"wins"`
	Losses     int `json:"losses"`
	Draws      int `json:"draws"`
	WinStreak  int `json:"win_streak"`
	BestStreak int `json:"best_streak"`
}

type Module struct {
	rooms *rooms.Registry
}

func NewModule() *Module {
	return &Module{rooms: rooms.NewRegistry()}
}

func (m *Module) Register(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, initializer runtime.Initializer) error {
	if err := initializer.RegisterMatch(matchName, func(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule) (runtime.Match, error) {
		return &tttMatch{module: m}, nil
	}); err != nil {
		return err
	}

	if err := initializer.RegisterRpc("create_room", m.rpcCreateRoom); err != nil {
		return err
	}
	if err := initializer.RegisterRpc("list_rooms", m.rpcListRooms); err != nil {
		return err
	}
	if err := initializer.RegisterRpc("join_room", m.rpcJoinRoom); err != nil {
		return err
	}
	if err := initializer.RegisterRpc("quick_match", m.rpcQuickMatch); err != nil {
		return err
	}
	if err := initializer.RegisterRpc("get_player_stats", m.rpcGetPlayerStats); err != nil {
		return err
	}
	if err := initializer.RegisterRpc("get_leaderboard", m.rpcGetLeaderboard); err != nil {
		return err
	}

	if err := ensureLeaderboard(ctx, nk); err != nil {
		logger.Warn("leaderboard create failed: %v", err)
	}

	logger.Info("tic-tac-toe module loaded")
	return nil
}

type tttMatch struct {
	module *Module
}

func (m *tttMatch) MatchInit(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, params map[string]interface{}) (interface{}, int, string) {
	mode := tictactoe.ModeClassic
	turnLimit := appmatch.DefaultTurnLimit

	if v, ok := params["mode"].(string); ok {
		mode = tictactoe.NormalizeMode(strings.TrimSpace(strings.ToLower(v)))
	}
	if v, ok := params["turn_limit"].(float64); ok && int(v) > 0 {
		turnLimit = int(v)
	}
	if v, ok := params["turn_limit"].(int); ok && v > 0 {
		turnLimit = v
	}
	if v, ok := params["turn_limit"].(int64); ok && v > 0 {
		turnLimit = int(v)
	}

	matchID := fmt.Sprint(ctx.Value(runtime.RUNTIME_CTX_MATCH_ID))
	state := appmatch.NewState(matchID, mode, turnLimit)
	now := time.Now().UTC()

	m.module.rooms.Upsert(rooms.Meta{
		MatchID:   state.MatchID,
		Mode:      state.Mode,
		Open:      true,
		Players:   0,
		CreatedAt: now,
		UpdatedAt: now,
	})

	label := mustJSON(map[string]interface{}{
		"mode":    state.Mode,
		"open":    true,
		"players": 0,
		"status":  state.Status,
	})
	return state, tickRate, label
}

func (m *tttMatch) MatchJoinAttempt(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, dispatcher runtime.MatchDispatcher, tick int64, state interface{}, presence runtime.Presence, metadata map[string]string) (interface{}, bool, string) {
	s := state.(*appmatch.State)
	if err := s.JoinAttempt(presence.GetUserId()); err != nil {
		return s, false, err.Error()
	}
	return s, true, ""
}

func (m *tttMatch) MatchJoin(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, dispatcher runtime.MatchDispatcher, tick int64, state interface{}, presences []runtime.Presence) interface{} {
	s := state.(*appmatch.State)
	joiners := make([]appmatch.Player, 0, len(presences))
	for _, p := range presences {
		joiners = append(joiners, appmatch.Player{UserID: p.GetUserId(), Username: p.GetUsername()})
	}
	s.Join(joiners, time.Now().UTC())
	m.updateRoomAndLabel(dispatcher, s)
	broadcastState(dispatcher, s)
	return s
}

func (m *tttMatch) MatchLeave(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, dispatcher runtime.MatchDispatcher, tick int64, state interface{}, presences []runtime.Presence) interface{} {
	s := state.(*appmatch.State)
	left := make([]string, 0, len(presences))
	for _, p := range presences {
		left = append(left, p.GetUserId())
	}
	s.Leave(left)
	if s.Ended {
		persistMatchOutcome(ctx, logger, nk, s)
	}
	m.updateRoomAndLabel(dispatcher, s)
	broadcastState(dispatcher, s)
	return s
}

func (m *tttMatch) MatchLoop(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, dispatcher runtime.MatchDispatcher, tick int64, state interface{}, messages []runtime.MatchData) interface{} {
	s := state.(*appmatch.State)

	if len(s.Players) == 0 {
		s.EmptyTicks++
		if s.EmptyTicks >= tickRate*30 {
			m.module.rooms.Delete(s.MatchID)
			return nil
		}
	}

	if s.Tick(time.Now().UTC()) {
		persistMatchOutcome(ctx, logger, nk, s)
		m.updateRoomAndLabel(dispatcher, s)
		broadcastState(dispatcher, s)
		return s
	}

	for _, msg := range messages {
		if msg.GetOpCode() != int64(moveOpCode) {
			continue
		}
		var move struct {
			Cell int `json:"cell"`
		}
		if err := json.Unmarshal(msg.GetData(), &move); err != nil {
			s.LastError = "invalid move payload"
			broadcastState(dispatcher, s)
			continue
		}

		finished, err := s.HandleMove(msg.GetUserId(), move.Cell, time.Now().UTC())
		if err != nil {
			broadcastState(dispatcher, s)
			continue
		}
		if finished {
			persistMatchOutcome(ctx, logger, nk, s)
		}
		m.updateRoomAndLabel(dispatcher, s)
		broadcastState(dispatcher, s)
	}
	return s
}

func (m *tttMatch) MatchTerminate(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, dispatcher runtime.MatchDispatcher, tick int64, state interface{}, graceSeconds int) interface{} {
	s := state.(*appmatch.State)
	m.module.rooms.Delete(s.MatchID)
	return s
}

func (m *tttMatch) MatchSignal(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, dispatcher runtime.MatchDispatcher, tick int64, state interface{}, data string) (interface{}, string) {
	return state, "signal not supported"
}

func broadcastState(dispatcher runtime.MatchDispatcher, s *appmatch.State) {
	_ = dispatcher.BroadcastMessage(100, mustJSONBytes(s.Snapshot()), nil, nil, true)
}

func (m *tttMatch) updateRoomAndLabel(dispatcher runtime.MatchDispatcher, s *appmatch.State) {
	meta, ok := m.module.rooms.Get(s.MatchID)
	if !ok {
		meta = rooms.Meta{MatchID: s.MatchID, Mode: s.Mode, CreatedAt: time.Now().UTC()}
	}
	meta.Players = len(s.Players)
	meta.Open = len(s.Players) < 2 && s.Status != tictactoe.StatusFinished
	meta.UpdatedAt = time.Now().UTC()
	m.module.rooms.Upsert(meta)

	_ = dispatcher.MatchLabelUpdate(mustJSON(map[string]interface{}{
		"mode":    s.Mode,
		"open":    meta.Open,
		"players": meta.Players,
		"status":  s.Status,
	}))
}

func (m *Module) rpcCreateRoom(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, payload string) (string, error) {
	var req struct {
		Mode      string `json:"mode"`
		TurnLimit int    `json:"turn_limit"`
	}
	if payload != "" {
		if err := json.Unmarshal([]byte(payload), &req); err != nil {
			return "", runtime.NewError("invalid payload", 3)
		}
	}

	mode := tictactoe.NormalizeMode(strings.TrimSpace(strings.ToLower(req.Mode)))
	turnLimit := req.TurnLimit
	if turnLimit <= 0 {
		turnLimit = appmatch.DefaultTurnLimit
	}

	matchID, err := nk.MatchCreate(ctx, matchName, map[string]interface{}{"mode": string(mode), "turn_limit": turnLimit})
	if err != nil {
		logger.Error("create room failed: %v", err)
		return "", runtime.NewError("unable to create room: "+err.Error(), 13)
	}
	return mustJSON(map[string]interface{}{"match_id": matchID, "mode": mode}), nil
}

func (m *Module) rpcListRooms(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, payload string) (string, error) {
	var req struct {
		Mode     string `json:"mode"`
		Limit    int    `json:"limit"`
		OnlyOpen bool   `json:"only_open"`
	}
	if payload != "" {
		if err := json.Unmarshal([]byte(payload), &req); err != nil {
			return "", runtime.NewError("invalid payload", 3)
		}
	}

	var mode tictactoe.Mode
	if req.Mode != "" {
		mode = tictactoe.NormalizeMode(strings.TrimSpace(strings.ToLower(req.Mode)))
	}
	if req.Limit <= 0 || req.Limit > 100 {
		req.Limit = 20
	}

	list := m.rooms.List(mode, req.OnlyOpen, req.Limit)
	return mustJSON(map[string]interface{}{"rooms": list}), nil
}

func (m *Module) rpcJoinRoom(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, payload string) (string, error) {
	var req struct {
		MatchID string `json:"match_id"`
	}
	if err := json.Unmarshal([]byte(payload), &req); err != nil {
		return "", runtime.NewError("invalid payload", 3)
	}
	if req.MatchID == "" {
		return "", runtime.NewError("match_id is required", 3)
	}

	room, ok := m.rooms.Get(req.MatchID)
	if !ok {
		return "", runtime.NewError("room not found", 5)
	}
	if !room.Open {
		return "", runtime.NewError("room is full or unavailable", 5)
	}

	return mustJSON(map[string]interface{}{"match_id": req.MatchID, "mode": room.Mode, "join_via": "socket.match_join"}), nil
}

func (m *Module) rpcQuickMatch(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, payload string) (string, error) {
	var req struct {
		Mode string `json:"mode"`
	}
	if payload != "" {
		if err := json.Unmarshal([]byte(payload), &req); err != nil {
			return "", runtime.NewError("invalid payload", 3)
		}
	}
	mode := tictactoe.NormalizeMode(strings.TrimSpace(strings.ToLower(req.Mode)))

	openRooms := m.rooms.List(mode, true, 1)
	if len(openRooms) > 0 {
		return mustJSON(map[string]interface{}{"match_id": openRooms[0].MatchID, "mode": openRooms[0].Mode, "created": false}), nil
	}

	matchID, err := nk.MatchCreate(ctx, matchName, map[string]interface{}{"mode": string(mode), "turn_limit": appmatch.DefaultTurnLimit})
	if err != nil {
		logger.Error("quick match create failed: %v", err)
		return "", runtime.NewError("unable to create match: "+err.Error(), 13)
	}
	return mustJSON(map[string]interface{}{"match_id": matchID, "mode": mode, "created": true}), nil
}

func (m *Module) rpcGetPlayerStats(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, payload string) (string, error) {
	userID := fmt.Sprint(ctx.Value(runtime.RUNTIME_CTX_USER_ID))
	if userID == "" {
		return "", runtime.NewError("unauthorized", 16)
	}
	stats, err := loadStats(ctx, nk, userID)
	if err != nil {
		return "", runtime.NewError("unable to load stats", 13)
	}
	return mustJSON(stats), nil
}

func (m *Module) rpcGetLeaderboard(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, payload string) (string, error) {
	var req struct {
		Limit int `json:"limit"`
	}
	if payload != "" {
		if err := json.Unmarshal([]byte(payload), &req); err != nil {
			return "", runtime.NewError("invalid payload", 3)
		}
	}
	if req.Limit <= 0 || req.Limit > 100 {
		req.Limit = 20
	}

	records, _, _, _, err := nk.LeaderboardRecordsList(ctx, leaderboardID, []string{}, req.Limit, "", 0)
	if err != nil {
		return "", runtime.NewError("unable to fetch leaderboard", 13)
	}
	return mustJSON(map[string]interface{}{"records": records}), nil
}

func persistMatchOutcome(ctx context.Context, logger runtime.Logger, nk runtime.NakamaModule, s *appmatch.State) {
	if len(s.Symbols) < 2 {
		return
	}

	userIDs := make([]string, 0, len(s.Symbols))
	for uid := range s.Symbols {
		userIDs = append(userIDs, uid)
	}
	if len(userIDs) != 2 {
		return
	}

	if s.WinnerUserID != "" {
		for _, uid := range userIDs {
			stats, err := loadStats(ctx, nk, uid)
			if err != nil {
				logger.Warn("load stats failed uid=%s err=%v", uid, err)
				continue
			}
			if uid == s.WinnerUserID {
				stats.Wins++
				stats.WinStreak++
				if stats.WinStreak > stats.BestStreak {
					stats.BestStreak = stats.WinStreak
				}
			} else {
				stats.Losses++
				stats.WinStreak = 0
			}
			if err := saveStats(ctx, nk, uid, stats); err != nil {
				logger.Warn("save stats failed uid=%s err=%v", uid, err)
				continue
			}
			upsertLeaderboard(ctx, logger, nk, uid, s, stats)
		}
		return
	}

	for _, uid := range userIDs {
		stats, err := loadStats(ctx, nk, uid)
		if err != nil {
			logger.Warn("load stats failed uid=%s err=%v", uid, err)
			continue
		}
		stats.Draws++
		stats.WinStreak = 0
		if err := saveStats(ctx, nk, uid, stats); err != nil {
			logger.Warn("save stats failed uid=%s err=%v", uid, err)
			continue
		}
		upsertLeaderboard(ctx, logger, nk, uid, s, stats)
	}
}

func upsertLeaderboard(ctx context.Context, logger runtime.Logger, nk runtime.NakamaModule, uid string, state *appmatch.State, stats playerStats) {
	score := int64(stats.Wins*3 + stats.Draws)
	subscore := int64(stats.WinStreak)
	meta := map[string]interface{}{"wins": stats.Wins, "losses": stats.Losses, "draws": stats.Draws, "win_streak": stats.WinStreak}
	username := ""
	if p, ok := state.Players[uid]; ok {
		username = p.Username
	}
	if _, err := nk.LeaderboardRecordWrite(ctx, leaderboardID, uid, username, score, subscore, meta, nil); err != nil {
		logger.Warn("leaderboard write failed uid=%s err=%v", uid, err)
	}
}

func loadStats(ctx context.Context, nk runtime.NakamaModule, userID string) (playerStats, error) {
	reads := []*runtime.StorageRead{{Collection: "ttt_stats", Key: "summary", UserID: userID}}
	objects, err := nk.StorageRead(ctx, reads)
	if err != nil {
		return playerStats{}, err
	}
	if len(objects) == 0 {
		return playerStats{}, nil
	}
	var out playerStats
	if err := json.Unmarshal([]byte(objects[0].Value), &out); err != nil {
		return playerStats{}, err
	}
	return out, nil
}

func saveStats(ctx context.Context, nk runtime.NakamaModule, userID string, stats playerStats) error {
	payload, _ := json.Marshal(stats)
	writes := []*runtime.StorageWrite{{
		Collection:      "ttt_stats",
		Key:             "summary",
		UserID:          userID,
		Value:           string(payload),
		PermissionRead:  2,
		PermissionWrite: 0,
	}}
	_, err := nk.StorageWrite(ctx, writes)
	return err
}

func ensureLeaderboard(ctx context.Context, nk runtime.NakamaModule) error {
	err := nk.LeaderboardCreate(ctx, leaderboardID, true, "desc", "best", "", map[string]interface{}{})
	if err != nil && !strings.Contains(strings.ToLower(err.Error()), "already exists") {
		return err
	}
	return nil
}

func mustJSON(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func mustJSONBytes(v interface{}) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}
