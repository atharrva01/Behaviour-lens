# BehaviourLens — Backend

Real-time user behaviour observability backend written in Go.

## Architecture

```
POST /events
     │
     ▼
 eventChannel (buffered chan, cap 1000)
     │
     ▼  (consumer goroutine)
 StateManager.ProcessEvent()   ← sliding window, O(1) trim
     │
     ▼
 RuleEngine.Evaluate()         ← hesitation · loop · abandonment
     │
     ▼
 ExplainPattern()              ← deterministic plain-English explanation
     │
     ├──► StateManager.StorePattern()   (in-memory ring buffer, max 500)
     └──► SSEHub.BroadcastPattern()     (all connected /api/stream clients)
```

## API Reference

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/health` | Liveness check |
| `POST` | `/events` | Ingest a user behaviour event |
| `GET` | `/api/patterns?limit=N` | Most recent N detected patterns (default 50) |
| `GET` | `/api/stats` | System metrics snapshot |
| `GET` | `/api/users/active?within=N` | Users active in the last N seconds (default 60) |
| `GET` | `/api/users/{id}/events` | Current sliding-window events for a user |
| `GET` | `/api/stream` | SSE stream — real-time `pattern` + `stats` events |

### Event Schema (POST /events)

```json
{
  "user_id": "usr_0001",
  "action": "idle",
  "page": "/checkout",
  "timestamp": 1700000000000,
  "metadata": { "duration_ms": "15000" }
}
```

Valid `action` values: `click`, `scroll`, `idle`, `navigate`, `abandon`, `tab_hidden`, `tab_visible`, `confirm`, `purchase`

### SSE Event Types

```
event: pattern
data: { ...Pattern }

event: stats
data: { ...SystemMetrics }
```

## Running Locally

### Prerequisites
- Go 1.23+

### Start the server

```powershell
cd "Behaviour lens/Backend"
go run ./cmd/server
```

Server starts on `:8080`.

### Run the simulator

```powershell
# 10 virtual users, 400ms average inter-event delay
go run ./cmd/simulator -users 10 -rate 400 -url http://localhost:8080

# Run for a fixed duration
go run ./cmd/simulator -users 5 -rate 300 -duration 2m
```

### Run tests

```powershell
go test ./...

# With race detector
go test -race ./...

# Verbose
go test -v ./...
```

## Docker

```powershell
# Build image
docker build -t behaviourlens-backend .

# Run
docker run -p 8080:8080 behaviourlens-backend

# Or with Compose
docker compose up
```

## Detected Patterns

| Pattern | Condition | Severity |
|---------|-----------|----------|
| `hesitation` | User idle ≥ 10s on current page | `low` / `medium` / `high` |
| `navigation-loop` | Same page visited ≥ 3× in window | `low` / `medium` / `high` |
| `abandonment` | User reached checkout depth ≥ 2 then navigated away | `high` |

All patterns fire at most once per 2-minute cooldown per user+page combination.

## Project Structure

```
Backend/
├── cmd/
│   ├── server/
│   │   ├── main.go         # Entry point, consumer goroutine, routing
│   │   ├── handlers.go     # HTTP handlers + CORS middleware
│   │   └── sse.go          # SSE hub + /api/stream handler
│   └── simulator/
│       └── main.go         # Load / demo simulator
├── internal/
│   ├── engine/
│   │   ├── rules.go        # Rule engine (detect)
│   │   ├── rules_test.go
│   │   ├── explain.go      # Explanation generator
│   └── state/
│       ├── manager.go      # StateManager (process, store, query)
│       └── manager_test.go
├── models/
│   ├── event.go
│   ├── pattern.go
│   ├── user_state.go
│   └── metrics.go
├── Dockerfile
├── docker-compose.yml
└── go.mod
```
