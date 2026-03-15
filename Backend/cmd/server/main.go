package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"behaviourlens/internal/engine"
	"behaviourlens/internal/models"
	"behaviourlens/internal/state"
)

// ── global singletons ─────────────────────────────────────────────────────────

var (
	eventChannel = make(chan models.Event, 1000)
	manager      = state.NewStateManager(5*time.Minute, 200)
	hub          = newSSEHub()
)

// ── health ────────────────────────────────────────────────────────────────────

func healthHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":    "ok",
		"timestamp": time.Now().UnixMilli(),
	})
}

// ── /events ───────────────────────────────────────────────────────────────────

func eventHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {

	case http.MethodPost:
		var e models.Event

		err := json.NewDecoder(r.Body).Decode(&e)
		if err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		if e.UserID == "" || e.Action == "" || e.Page == "" || e.Timestamp == 0 {
			http.Error(w, "Missing required fields", http.StatusBadRequest)
			return
		}

		if !models.ValidActions[e.Action] {
			http.Error(w, "Invalid action: must be one of click, scroll, idle, navigate, abandon, tab_hidden, tab_visible, confirm, purchase", http.StatusBadRequest)
			return
		}

		select {
		case eventChannel <- e:
			writeJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
		default:
			http.Error(w, "Server overloaded", http.StatusServiceUnavailable)
		}

	case http.MethodGet:
		userID := r.URL.Query().Get("user_id")
		if userID == "" {
			http.Error(w, "user_id query param required", http.StatusBadRequest)
			return
		}

		events, exists := manager.GetUserEvents(userID)
		if !exists {
			http.Error(w, "No events found for user", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(events)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// ── /api/users/{id}/events ────────────────────────────────────────────────────

// userEventsHandler serves GET /api/users/{id}/events.
// It extracts the user ID from the URL path and returns the current event window.
func userEventsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Path: /api/users/{id}/events  →  trim prefix and suffix to get id.
	path := strings.TrimPrefix(r.URL.Path, "/api/users/")
	path = strings.TrimSuffix(path, "/events")
	userID := strings.TrimSpace(path)

	if userID == "" {
		http.Error(w, "user_id missing from path", http.StatusBadRequest)
		return
	}

	events, exists := manager.GetUserEvents(userID)
	if !exists {
		http.Error(w, "No events found for user", http.StatusNotFound)
		return
	}

	writeJSON(w, http.StatusOK, events)
}

// ── consumer goroutine ────────────────────────────────────────────────────────

// startConsumer drains the event channel, updates state, runs the rule engine,
// generates explanations, stores patterns, and broadcasts new patterns via SSE.
func startConsumer() {
	ruleEngine := engine.NewRuleEngine()

	go func() {
		for event := range eventChannel {
			// ProcessEvent returns a safe snapshot of the updated user state.
			snapshot := manager.ProcessEvent(event)

			// Run all behavioral rules against the snapshot.
			// This happens outside any lock — the snapshot is an independent copy.
			patterns := ruleEngine.Evaluate(snapshot)

			// Attach a plain-English explanation to each pattern, persist, and broadcast.
			for _, p := range patterns {
				p.Explanation = engine.ExplainPattern(p, snapshot)
				manager.StorePattern(p)
				hub.BroadcastPattern(p)
				log.Printf("pattern detected: type=%s user=%s page=%s severity=%s | %s",
					p.Type, p.UserID, p.Page, p.Severity, p.Explanation)
			}
		}
	}()
}

// ── entry point ───────────────────────────────────────────────────────────────

func main() {
	startConsumer()

	// Start broadcasting SystemMetrics over SSE every 5 seconds.
	hub.StartStatsTicker(5*time.Second, manager.GetMetrics)

	mux := http.NewServeMux()

	// Core ingestion
	mux.HandleFunc("/health", healthHandler)
	mux.HandleFunc("/events", eventHandler)

	// Pattern & stats queries
	mux.HandleFunc("/api/patterns", patternsHandler)
	mux.HandleFunc("/api/stats", statsHandler)

	// User queries
	mux.HandleFunc("/api/users/active", activeUsersHandler)
	mux.HandleFunc("/api/users/", userEventsHandler) // matches /api/users/{id}/events

	// Real-time SSE stream
	mux.HandleFunc("/api/stream", streamHandler(hub))

	server := &http.Server{
		Addr:    ":8080",
		Handler: corsMiddleware(mux), // CORS applied globally
	}

	log.Println("BehaviourLens server started on :8080")
	log.Fatal(server.ListenAndServe())
}
