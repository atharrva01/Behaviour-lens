# Schemas And API Notes

## Event

```json
{
  "user_id": "usr_0001",
  "action": "idle",
  "page": "/checkout",
  "timestamp": 1700000000000,
  "metadata": {
    "duration_ms": "15000"
  }
}
```

Fields:

- `user_id`: unique user identifier
- `action`: `click`, `scroll`, `idle`, `navigate`, `abandon`, `tab_hidden`, `tab_visible`, `confirm`, `purchase`
- `page`: route or page identifier
- `timestamp`: epoch milliseconds
- `metadata`: optional rule-specific details

## UserState

- `UserID`
- `Events`
- `CurrentPage`
- `LastSeen`
- `SessionStart`
- `PageVisitCounts`
- `TabVisible`
- `CheckoutDepth`

This state lives only in memory and is bounded by window duration plus max events per user.

## Pattern

```json
{
  "pattern_id": "usr_0001_hesitation_1700000001000",
  "user_id": "usr_0001",
  "type": "hesitation",
  "page": "/checkout",
  "detected_at": 1700000001000,
  "explanation": "User paused for 15s on /checkout without taking action.",
  "severity": "medium",
  "resolved": false
}
```

Types:

- `hesitation`
- `navigation-loop`
- `abandonment`

## SystemMetrics

```json
{
  "total_events": 1204,
  "active_users": 18,
  "patterns_detected": 46,
  "abandonment_rate": 0.17,
  "as_of": 1700000002000
}
```

## Backend Endpoints

- `GET /health`
- `POST /events`
- `GET /api/patterns?limit=N`
- `GET /api/stats`
- `GET /api/users/active?within=N`
- `GET /api/users/{id}/events`
- `GET /api/stream`

## Detection Rules

- Hesitation: idle duration at least 10 seconds on the current page
- Navigation loop: same page visited at least 3 times in the current window
- Abandonment: checkout depth at least 2, then abandon or navigate away without completion
