# BehaviourLens

**Real-time user behavior observability ,  see friction as it forms, not after it causes churn.**

Most analytics tools tell you what happened yesterday. BehaviourLens tells you what's happening right now. It watches live user sessions, detects behavioral friction signals like hesitation, navigation loops, and checkout abandonment the moment they occur, and explains them in plain English on a live dashboard, while the user is still on the page.

Built as a production-inspired system using Go, React, and Server-Sent Events. No external databases, no third-party services, no magic , just a clean event pipeline you can read end to end.

<img width="1920" height="1020" alt="image" src="https://github.com/user-attachments/assets/6ba5b33f-31da-4a88-9681-f13258394869" />

## What It Does

You send it user events (clicks, scrolls, navigation, idle time). It processes them through a sliding-window state engine, runs three behavioral detection rules, generates a plain-English explanation for each signal, and streams everything to a live dashboard over SSE — all in under a millisecond of processing time.

Three things it detects:

| Pattern | What it means | Example |
|---|---|---|
| **Hesitation** | User went idle on a page for too long | Sitting on `/checkout` for 25 seconds without interacting |
| **Navigation loop** | User keeps bouncing between the same pages | Going `/search` → `/products` → `/search` → `/products` three or more times |
| **Abandonment** | User reached the checkout flow then left | Got to `/payment`, then navigated back to `/home` |

Each detection includes a severity level (low / medium / high), the exact user and page it happened on, and a human-readable explanation with real numbers pulled from the event window.

---

## How It Works (the short version)

```
Your website or demo store
        │
        │  POST /events  (click, scroll, idle, navigate…)
        ▼
  ┌─────────────────────────────────────────────────────┐
  │  Go Backend                                          │
  │                                                      │
  │  [Buffered Channel] → [State Manager] → [Rule Engine]│
  │                              │                       │
  │                    Sliding window per user           │
  │                    (5 min, max 200 events)           │
  │                              │                       │
  │                       [Explain Engine]               │
  │                     rule-based or Claude AI          │
  │                              │                       │
  │                    [SSE Hub] broadcasts live         │
  └──────────────────────────────┬──────────────────────┘
                                 │
                    event: pattern / event: stats
                                 │
                                 ▼
                       React Dashboard (localhost:5173)
                       Live pattern feed, metrics,
                       active sessions, event inspector
```

The HTTP layer accepts events and returns immediately (`202 Accepted`). A background consumer goroutine does all the processing  the pipeline never blocks the ingest path. State is protected with a `sync.RWMutex` and the rule engine always works on a deep-copied snapshot, so there are zero lock contention issues between ingestion and detection.

---

## Project Structure

```
Behaviour lens/
├── Backend/
│   ├── cmd/
│   │   ├── server/
│   │   │   ├── main.go         # Entry point, graceful shutdown, AI wiring
│   │   │   ├── handlers.go     # HTTP handlers, CORS, severity filter, resolve endpoint
│   │   │   ├── sse.go          # Server-Sent Events hub
│   │   │   ├── demo.go         # Serves the built-in demo store
│   │   │   └── demo.html       # Self-contained e-commerce demo (tracker pre-installed)
│   │   └── simulator/
│   │       └── main.go         # Virtual user load simulator (5 scenarios)
│   └── internal/
│       ├── engine/
│       │   ├── rules.go        # Detection rules (hesitation, loop, abandonment)
│       │   ├── rules_test.go   # 18 tests covering all rule logic
│       │   ├── explain.go      # Rule-based explanation generator
│       │   └── ai_explain.go   # Optional Claude AI explanation layer
│       ├── state/
│       │   ├── manager.go      # StateManager — sliding window, pattern store, metrics
│       │   └── manager_test.go # 16 tests covering state management
│       └── models/
│           ├── event.go        # Event struct + valid action types
│           ├── pattern.go      # Pattern struct + type/severity constants
│           ├── user_state.go   # Per-user session state
│           └── metrics.go      # SystemMetrics snapshot
├── Frontend/
│   └── src/
│       ├── App.jsx             # Dashboard — metrics, feed, sessions, event inspector
│       ├── styles.css          # Clean dark design system
│       └── api.js              # Fetch wrapper
└── README.md
```

---

## Running Locally

### Prerequisites

- **Go** 1.23 or later → [golang.org/dl](https://golang.org/dl/)
- **Node.js** 18 or later → [nodejs.org](https://nodejs.org/)
- A terminal (PowerShell, bash, zsh — any works)

---

### Step 1 — Clone the repo

```bash
git clone https://github.com/atharrva01/Behaviour-lens.git
cd "Behaviour lens"
```

---

### Step 2 — Start the backend

```bash
cd Backend
go run ./cmd/server
```

You should see:

```
2026/03/23 10:00:00 AI explanation disabled  using rule-based explanations
2026/03/23 10:00:00 BehaviourLens server started on :8080
```

Verify it's alive:

```bash
curl http://localhost:8080/health
# → {"status":"ok","timestamp":1234567890000}
```

The server runs on **port 8080** by default. To change it:

```bash
PORT=9090 go run ./cmd/server
```

---

### Step 3 — Start the frontend

Open a second terminal:

```bash
cd Frontend
npm install      # first time only
npm run dev
```

You should see Vite start up and print something like:

```
  VITE v5.4.10  ready in 312 ms
  ➜  Local:   http://localhost:5173/
```

Open **http://localhost:5173** in your browser. The dashboard will load and show a green **live** status dot once it connects to the backend stream.

---

### Step 4 — Generate traffic (two options)

#### Option A  Built-in demo store (recommended for demos)

Open **http://localhost:8080/demo** in a second browser tab. This is a fully functional mini e-commerce store called *Verse* with the BehaviourLens tracker already embedded.

Browse it like a real user:
- Sit on the `/checkout` page without doing anything for 15 seconds → watch a **hesitation** pattern appear
- Navigate between Products and Search repeatedly → watch a **navigation-loop** fire
- Add something to cart, reach the payment page, then click the logo to go home → watch **abandonment** trigger
- Complete a full purchase → no pattern fires (successful flow)

The purple bar at the bottom of the demo store shows your session ID, current page, and the last event that fired. Click **→ open dashboard** to jump to the dashboard tab.

#### Option B — Simulator (for load testing and continuous data)

```bash
# Default: 10 virtual users, 500ms between events, runs forever
cd Backend
go run ./cmd/simulator

# Custom: 20 users, faster events, run for 3 minutes
go run ./cmd/simulator -users 20 -rate 300 -duration 3m

# Point at a different backend
go run ./cmd/simulator -url http://localhost:9090
```

The simulator runs 5 pre-scripted scenarios in parallel across all virtual users:
- **Normal browse** — random pages, no friction
- **Hesitation** — idles 10–45 seconds on a page
- **Navigation loop** — bounces between two pages 3–5 times
- **Checkout abandonment** — walks into the funnel and bails
- **Full purchase** — completes the entire checkout flow

---

### Step 5 — Using the dashboard

| Section | What it shows |
|---|---|
| **Metrics bar** | Total events, active users, patterns detected, abandonment rate — updates every 5 seconds via SSE |
| **Latest alert** | The most recent pattern, always pinned at the top of the feed |
| **Live pattern feed** | All detected patterns as a scrollable list, newest first |
| **Type filter** | Filter by hesitation / navigation-loop / abandonment |
| **Severity filter** | Filter by high / medium / low |
| **Search** | Full-text search across user ID, page, and explanation |
| **Active sessions** | Every user seen in the last 60 seconds — click any to inspect |
| **Event window** | The selected user's last N events, live-updating |
| **Resolve button** | Mark a pattern as resolved — it dims and gets re-broadcast to all dashboard clients |

---

## API Reference

All endpoints return JSON. CORS is open (`*`) for local development.

### Ingest

```
POST /events
Content-Type: application/json

{
  "user_id":   "usr_0001",
  "action":    "idle",
  "page":      "/checkout",
  "timestamp": 1700000000000,
  "metadata":  { "duration_ms": "18000" }
}
```

Valid `action` values: `click` `scroll` `idle` `navigate` `abandon` `tab_hidden` `tab_visible` `confirm` `purchase`

Returns `202 Accepted` immediately. The event is queued and processed asynchronously.

---

### Query

```bash
# Detected patterns (most recent first)
GET /api/patterns
GET /api/patterns?limit=20
GET /api/patterns?severity=high

# System metrics
GET /api/stats

# Active users (seen in last 60s)
GET /api/users/active
GET /api/users/active?within=120

# Specific user's event window
GET /api/users/{user_id}/events

# Mark a pattern resolved
PATCH /api/patterns/{pattern_id}/resolve

# Health check
GET /health
```

---

### Real-time stream

```bash
# Connect to the SSE stream (stays open)
curl -N http://localhost:8080/api/stream
```

Two event types flow through the stream:

```
event: pattern
data: {"pattern_id":"usr_001_hesitation_...","type":"hesitation","page":"/checkout","severity":"medium","explanation":"User paused for 18s...","user_id":"usr_001","detected_at":1700000000000,"resolved":false}

event: stats
data: {"total_events":1240,"active_users":7,"patterns_detected":23,"abandonment_rate":0.14,"as_of":1700000000000}
```

Stats broadcast every 5 seconds. Patterns broadcast the moment they're detected.

---

## Optional — AI-Powered Explanations

By default, pattern explanations are generated by a deterministic rule-based engine (fast, offline, auditable). If you want richer natural-language explanations, you can plug in Claude:

```bash
ANTHROPIC_API_KEY=sk-ant-... go run ./cmd/server
```

When the key is set you'll see:

```
2026/03/23 10:00:00 AI explanation layer enabled (claude-haiku)
```

The AI only explains patterns — it never detects them. Rule-based detection always runs first. If the API call fails or times out (3 second ceiling), the system silently falls back to rule-based explanation. Detection is never blocked.

This is intentional. Non-deterministic detection is hard to audit and introduces latency. Rule-based detection fires in under 1ms. The AI is a UX layer, not a logic layer.

---

## Running Tests

```bash
cd Backend

# Run all tests
go test ./...

# With race detector (recommended)
go test -race ./...

# Verbose output
go test -v ./...
```

35 tests across the engine and state packages. All should pass in under a second.

---

## Environment Variables

| Variable | Default | Description |
|---|---|---|
| `PORT` | `8080` | Port the backend listens on |
| `ANTHROPIC_API_KEY` | _(unset)_ | Enable AI explanations via Claude Haiku |

---

## Embedding BehaviourLens on Your Own Site

The demo store at `/demo` shows how the tracker works. To track a real website you own, add this snippet before your closing `</body>` tag:

```html
<script>
(function() {
  const USER_ID = sessionStorage.getItem('bl_uid') || (() => {
    const id = 'usr_' + Math.random().toString(36).slice(2, 10);
    sessionStorage.setItem('bl_uid', id);
    return id;
  })();

  let currentPage = window.location.pathname;

  function track(action, meta) {
    fetch('http://YOUR_BACKEND_URL/events', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        user_id: USER_ID,
        action: action,
        page: currentPage,
        timestamp: Date.now(),
        ...(meta ? { metadata: meta } : {})
      })
    }).catch(() => {});
  }

  // Navigation
  window.addEventListener('popstate', () => {
    currentPage = window.location.pathname;
    track('navigate');
  });

  // Idle detection
  let lastAct = Date.now(), idleFired = false;
  setInterval(() => {
    const ms = Date.now() - lastAct;
    if (ms > 8000 && !idleFired) { track('idle', { duration_ms: String(ms) }); idleFired = true; }
  }, 2000);
  ['mousemove','keydown','touchstart'].forEach(e =>
    document.addEventListener(e, () => { lastAct = Date.now(); idleFired = false; }, { passive: true })
  );

  // Tab visibility
  document.addEventListener('visibilitychange', () =>
    track(document.hidden ? 'tab_hidden' : 'tab_visible')
  );

  track('navigate'); // initial page load
})();
</script>
```

Replace `YOUR_BACKEND_URL` with wherever your backend is running.

---

## Tech Stack

| Layer | Tech | Why |
|---|---|---|
| Backend | Go 1.23, `net/http` | Fast, no framework overhead, goroutines are perfect for the pipeline |
| Real-time | Server-Sent Events | Simpler than WebSockets for one-directional push; works through proxies |
| State | In-memory maps + `sync.RWMutex` | No database needed for MVP; bounded memory by design |
| Frontend | React 18 + Vite | `startTransition` + `useDeferredValue` keep UI responsive under heavy load |
| AI (optional) | Anthropic Claude Haiku | Fastest, cheapest Claude model; 3s timeout with rule-based fallback |

---

## Built By

**Atharva Ramesh Borade** 
1st Year,  Product Developer track

This project was built to mirror how real companies build streaming observability systems, at a smaller, explainable scale. Every architectural decision (buffered channels, snapshot isolation, cooldown windows, SSE over WebSockets) was made for a reason that you could explain in an interview.
