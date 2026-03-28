# Tic-Tac-Toe Nakama Backend

Server-authoritative multiplayer Tic-Tac-Toe backend built as a Nakama Go runtime module, with a local React test client.

## Deployed Endpoints

- Web UI: `http://136.119.253.104/`
- Nakama API/WebSocket: `http://136.119.253.104:7350`

## Scope

Implemented:
- Authoritative game state and move validation on server
- Matchmaking RPCs (`create_room`, `list_rooms`, `join_room`, `quick_match`)
- Real-time state broadcast over match socket
- Timed mode (turn timeout forfeit)
- Disconnect handling (remaining player wins)
- Player stats persistence + leaderboard updates
- Concurrent match sessions (isolated per match)

## Architecture

Layered modular monolith:
- `main.go`: composition root (`InitModule`)
- `internal/domain/tictactoe`: pure game rules
- `internal/domain/rooms`: room registry model/service
- `internal/application/match`: match lifecycle/use-case logic
- `internal/infrastructure/nakama`: Nakama adapter (RPCs, match hooks, storage, leaderboard)

## Repository Layout

- `Dockerfile`: builds `backend.so` plugin and Nakama image
- `docker-compose.yml`: Nakama + Postgres local stack
- `local.yml`: Nakama config
- `web/`: local React test UI

## Prerequisites

- Docker Engine + Docker Compose v2
- Optional: Go 1.22 (only if building/testing outside Docker)

## Local Run (Backend)

```bash
docker compose down -v
docker compose build --no-cache nakama
docker compose up -d
docker compose logs -f nakama
```

Backend is ready when logs show `Startup done`.

Endpoints:
- API/WebSocket: `http://localhost:7350`
- gRPC: `localhost:7349`
- Console: `http://localhost:7351`

## Local Run (Web Test UI)

Containerized web UI (recommended on VM):

```bash
docker compose build web
docker compose up -d web
```

Open `http://<server-ip>/`

Non-containerized web UI:

```bash
cd web
npm install
npm run dev
```

Open `http://localhost:5173` and connect with:
- Host: `127.0.0.1`
- Port: `7350`
- Server key: `supersecrettestkey`
- SSL: disabled

UI supports create/list/join/quick-match, gameplay, and optional bot opponent.

## How To Play Multiplayer

### Two real players

1. Open the web app in two separate browsers/devices.
2. In both clients, connect to the same Nakama endpoint (host, port `7350`, server key, SSL setting).
3. Player A clicks `Create Room` (or `Quick Match`).
4. Player B clicks `Quick Match` (or `List Rooms` then `Join`).
5. Both players play turns on the same board; server enforces move validity and turn order.

### Solo testing with bot

1. Connect one client.
2. Click `Quick Match` (or `Create Room`).
3. Enable `Enable Bot Opponent`.
4. Play normally; bot auto-joins as second player and plays on its turns.

### Timed mode

1. Set mode to `Timed`.
2. Start match via `Quick Match` or `Create Room`.
3. If current player does not move before `move_deadline_unix`, server auto-forfeits that player.

## How Evaluator Can Test

1. Open the deployed web UI URL.
2. In Connection panel, set:
   - Host: `136.119.253.104`
   - Port: `7350`
   - Server key: `supersecrettestkey`
   - SSL: off
3. Click `Connect`.
4. Click `Quick Match`.
5. Verify board state appears and moves are accepted only on your turn.
6. Enable `Enable Bot Opponent` and verify bot joins and responds with moves.
7. Switch mode to `Timed`, start a new match, wait out one turn, verify timeout forfeit.
8. Optional: call `get_player_stats` / `get_leaderboard` and verify updates after game end.

## RPC Contract

### `create_room`
Request:
```json
{"mode":"classic","turn_limit":30}
```
Response:
```json
{"match_id":"<id>","mode":"classic"}
```

### `list_rooms`
Request:
```json
{"mode":"classic","limit":20,"only_open":true}
```
Response:
```json
{"rooms":[{"match_id":"...","mode":"classic","open":true,"players":1,"created_at":"...","updated_at":"..."}]}
```

### `join_room`
Request:
```json
{"match_id":"<id>"}
```
Response:
```json
{"match_id":"<id>","mode":"classic","join_via":"socket.match_join"}
```

### `quick_match`
Request:
```json
{"mode":"timed"}
```
Response:
```json
{"match_id":"<id>","mode":"timed","created":false}
```

### `get_player_stats`
Response:
```json
{"wins":3,"losses":1,"draws":2,"win_streak":1,"best_streak":2}
```

### `get_leaderboard`
Request:
```json
{"limit":20}
```
Response:
```json
{"records":[...]}
```

## Match Socket Protocol

- Match handler: `tic_tac_toe_match`
- Client move opcode: `1`
- Client move payload:
```json
{"cell":0}
```
- Server state opcode: `100`
- Server state payload includes:
  - `board`, `status`, `turn_user_id`, `winner_user_id`, `players`
  - `move_deadline_unix` in timed mode

## Tests

```bash
go test ./...
```

Current coverage focus:
- domain game rules
- room registry filtering/order
- application match flow (join/move/win/timeout)

## Deployment (VM)

Recommended for assignment reliability: VM-based deployment (GCP/AWS/DO).

Minimal steps:
1. Install Docker + Compose on VM
2. Clone repo
3. Run backend commands from **Local Run (Backend)**
4. Expose `7350` publicly
5. Restrict `7351` to admin IP only

## Security Notes

For non-local environments, change defaults in `local.yml`:
- `socket.server_key`
- `console.username`
- `console.password`
- session/runtime keys

## Version Compatibility Note

Go plugins in Nakama require exact dependency compatibility with the Nakama binary.
This repo is pinned for Nakama `3.22.0` compatibility:
- `github.com/heroiclabs/nakama-common v1.32.0`
- `google.golang.org/protobuf v1.34.1`
