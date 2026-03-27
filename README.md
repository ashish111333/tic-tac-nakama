# Tic-Tac-Toe Nakama Backend

Production-ready Nakama backend for multiplayer Tic-Tac-Toe with server-authoritative rules, room/matchmaking RPCs, timed mode, and leaderboard/stat tracking.

## What Is Implemented

- Server-authoritative Tic-Tac-Toe match handler in Go (`main.go`)
- Full move validation on server (turn, bounds, occupancy, match state)
- Real-time state broadcast to all connected players
- Room creation/discovery/join RPC endpoints
- Quick matchmaking RPC (join open room or create one)
- Graceful disconnect handling (opponent wins on disconnect in active game)
- Concurrent room support with isolation per match state
- Optional timed mode (default 30s per turn, timeout forfeit)
- Persistent player stats in Nakama storage (`wins/losses/draws/streak`)
- Global leaderboard updates on match completion
- Dockerized local setup and cloud deployment guide

## Project Structure

- `main.go`: Thin entrypoint (Nakama `InitModule`)
- `internal/domain/tictactoe`: Core game rules/engine (pure domain logic)
- `internal/domain/rooms`: Room registry entity/service
- `internal/application/match`: Match orchestration use-case layer
- `internal/infrastructure/nakama`: Nakama adapter (RPCs, match hooks, persistence wiring)
- `Dockerfile`: Builds Go plugin and packages custom Nakama image
- `docker-compose.yml`: Local Nakama + Postgres stack
- `local.yml`: Nakama runtime config
- `go.mod` / `go.sum`: Go module dependencies

## Requirements

- Docker + Docker Compose
- Optional local Go 1.22+ (only needed if building plugin outside Docker)

## Local Setup

1. Start stack:

```bash
docker compose up --build -d
```

2. Check logs:

```bash
docker compose logs -f nakama
```

3. Endpoints:

- HTTP API: `http://localhost:7350`
- gRPC: `localhost:7349`
- Console: `http://localhost:7351` (`admin` / `password`)

4. Stop:

```bash
docker compose down
```

## Local Web UI (React)

A local test client is included in [`web/`](/home/ash/tic-tac-nakama/web).

1. Start Nakama backend first:

```bash
docker compose up --build -d
```

2. Start web client:

```bash
cd web
npm install
npm run dev
```

3. Open `http://localhost:5173`
4. Connect using:
- Host: `127.0.0.1`
- Port: `7350`
- Server key: `supersecrettestkey`
- SSL: unchecked (for local)

The UI supports room creation, quick match, room listing, move sending, and includes an optional in-browser bot opponent (`Enable Bot Opponent`) for solo testing.

## Architecture and Design Decisions

### 1) Clean Architecture Boundaries

- Domain: Tic-Tac-Toe rules and room models with no Nakama dependency
- Application: Match lifecycle orchestration and policy (join/leave/move/timeout)
- Infrastructure: Nakama runtime handlers, RPC transport, storage/leaderboard IO
- Entrypoint: minimal composition root in `main.go`

### 2) Authoritative Match Logic

All game state is maintained only in the server match state (`tttMatchState`).
Clients send move intents (opcode `1`), server validates and applies or rejects.

Validation includes:

- Match is in progress
- Correct player's turn
- Cell index is 0..8
- Cell is unoccupied

### 3) Match/Room Isolation for Concurrency

Each Nakama match instance owns its own board/players/turn/timer. This allows multiple simultaneous games without state crossover.

### 4) Room Discovery + Matchmaking

`roomRegistry` tracks active rooms in memory:

- `create_room`: creates authoritative match
- `list_rooms`: discovers available rooms
- `join_room`: preflight room check before socket join
- `quick_match`: auto-joins first open room in mode or creates one

### 5) Disconnect Handling

If a player disconnects during an active game and one player remains, remaining player is declared winner and stats/leaderboard are updated.

### 6) Timed Mode

If `mode=timed`, each turn has deadline (`move_deadline_unix`) and timeout causes automatic forfeit.

### 7) Persistence

- Player stats stored in collection `ttt_stats`, key `summary`
- Leaderboard ID: `ttt_global`
- Score formula: `wins*3 + draws`

## RPC / Server Configuration Details

RPC names registered in Nakama:

### `create_room`
Request:

```json
{"mode":"classic"}
```

Optional: `mode` = `classic|timed`, `turn_limit` (seconds)

Response:

```json
{"match_id":"<nakama-match-id>","mode":"classic"}
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
{"match_id":"<nakama-match-id>"}
```

Response:

```json
{"match_id":"...","mode":"classic","join_via":"socket.match_join"}
```

### `quick_match`
Request:

```json
{"mode":"timed"}
```

Response:

```json
{"match_id":"...","mode":"timed","created":false}
```

### `get_player_stats`
Request payload can be empty.

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

## Real-Time Match Protocol

- Match handler name: `tic_tac_toe_match`
- Client joins using socket `match_join(match_id)`
- Move opcode from client: `1`
  - Payload: `{"cell": 0}`
- Server state broadcast opcode: `100`
  - Payload includes board, players, turn, winner, status, optional timer deadline

## How To Test Multiplayer

1. Start backend with Docker Compose.
2. Create two users and authenticate from two clients/devices.
3. Player A calls `create_room` or `quick_match`.
4. Player B calls `list_rooms` or `quick_match`, then `match_join` with returned `match_id`.
5. Exchange move messages via opcode `1`.
6. Verify:
   - Invalid moves are rejected
   - Turn order enforced
   - Win/draw detection works
   - Disconnect awards win to remaining player
   - Timed mode forfeits on timeout
   - Stats and leaderboard update after game end

## Deployment Process Documentation

### Option A: DigitalOcean Droplet (quick and cost-effective)

1. Create Ubuntu droplet (2 vCPU / 4 GB RAM recommended).
2. Install Docker and Compose plugin.
3. Copy repository to server.
4. Set environment-specific secrets in `local.yml`:
   - `socket.server_key`
   - `console.username/password`
5. Launch:

```bash
docker compose up --build -d
```

6. Open firewall ports:
   - `7350` (HTTP/WebSocket)
   - `7349` (gRPC, optional)
   - `7351` (console; restrict by IP or VPN)

7. Put an HTTPS reverse proxy (Nginx/Caddy/Traefik) in front of `7350`.
8. Point frontend/mobile app to public endpoint.

### Option B: AWS/GCP/Azure

Use same containerized stack on VM or container service.
Recommended production extras:

- Managed PostgreSQL
- TLS termination at load balancer
- Private network between Nakama and DB
- Autoscaling with shared DB
- Centralized logging + metrics

## Frontend Deployment Note

Deploy frontend separately (Vercel/Netlify/Firebase Hosting or mobile build distribution).
Configure it with your public Nakama endpoint (`wss://<domain>/ws` via `7350`).

## Build Plugin Locally (Optional)

```bash
go build -buildmode=plugin -o backend.so .
```

## Run Tests

```bash
go test ./...
```

## Security / Hardening Checklist

- Rotate server and console credentials
- Restrict console (`7351`) to admin network
- Enable HTTPS/WSS in production
- Add rate limiting at proxy/load balancer
- Back up PostgreSQL

## Deliverables Mapping

- Source code repository: this project
- Deployed Nakama endpoint: follow deployment section above
- Public frontend/mobile app: integrate with this backend endpoint
- README includes setup, architecture, deployment, config, and multiplayer testing
