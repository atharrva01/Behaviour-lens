# Architecture

## High-Level Design

```mermaid
flowchart LR
    A["User or Simulator"] --> B["POST /events"]
    B --> C["Buffered channel"]
    C --> D["StateManager"]
    D --> E["RuleEngine"]
    E --> F["ExplainPattern"]
    F --> G["Pattern store"]
    F --> H["SSE hub"]
    G --> I["GET /api/patterns"]
    D --> J["GET /api/stats"]
    D --> K["GET /api/users/active"]
    D --> L["GET /api/users/{id}/events"]
    H --> M["React dashboard"]
    I --> M
    J --> M
    K --> M
    L --> M
```

## Low-Level Design

```mermaid
flowchart TD
    A["eventHandler"] --> B["eventChannel chan Event"]
    B --> C["consumer goroutine"]
    C --> D["manager.ProcessEvent(event)"]
    D --> E["deep-copied UserState snapshot"]
    E --> F["ruleEngine.Evaluate(snapshot)"]
    F --> G["[]Pattern"]
    G --> H["ExplainPattern(pattern, snapshot)"]
    H --> I["manager.StorePattern(pattern)"]
    H --> J["hub.BroadcastPattern(pattern)"]
    J --> K["SSE clients"]
    C --> L["metrics ticker"]
    L --> M["hub.BroadcastStats(metrics)"]
```

## Engineering Sequence

```mermaid
sequenceDiagram
    participant Simulator
    participant IngestAPI
    participant Channel
    participant StateManager
    participant RuleEngine
    participant Dashboard

    Simulator->>IngestAPI: POST /events {user_id, action, page, timestamp}
    IngestAPI->>Channel: push(Event)
    Channel->>StateManager: consume(Event)
    StateManager->>StateManager: update sliding window
    StateManager->>RuleEngine: evaluate(UserState snapshot)
    RuleEngine-->>StateManager: []Pattern
    StateManager->>Dashboard: SSE pattern / stats events
    Dashboard->>IngestAPI: GET /api/patterns
    Dashboard->>IngestAPI: GET /api/stats
    Dashboard->>IngestAPI: GET /api/users/active
```

## Consumer Flow

### Product flow

```mermaid
flowchart LR
    A["User lands on checkout"] --> B["User idles or loops"]
    B --> C["Backend detects behavioral pattern"]
    C --> D["Explanation generated"]
    D --> E["Pattern card appears on dashboard"]
    E --> F["PM / UX / Engineer investigates"]
```

### Key tradeoffs

- In-memory state keeps the MVP simple and fast, but resets on restart.
- Rule-based detection is deterministic and interview-friendly, but less adaptive than ML.
- SSE is lighter than WebSockets for this one-way live feed.
