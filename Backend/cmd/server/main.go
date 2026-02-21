package main

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"behaviourlens/internal/engine"
	"behaviourlens/internal/models"
	"behaviourlens/internal/state"
)

var (
	eventChannel = make(chan models.Event, 1000)
	manager      = state.NewStateManager(5*time.Minute, 200)
)

func healthHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":    "ok",
		"timestamp": time.Now().UnixMilli(),
	})
}

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
			http.Error(w, "Invalid action: must be one of click, scroll, idle, navigate, abandon", http.StatusBadRequest)
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

func startConsumer() {
	ruleEngine := engine.NewRuleEngine()

	go func() {
		for event := range eventChannel {
			// ProcessEvent returns a safe snapshot of the updated user state.
			snapshot := manager.ProcessEvent(event)

			// Run all behavioral rules against the snapshot.
			// This happens outside any lock — the snapshot is an independent copy.
			patterns := ruleEngine.Evaluate(snapshot)

			// Attach a plain-English explanation to each pattern, then persist.
			for _, p := range patterns {
				p.Explanation = engine.ExplainPattern(p, snapshot)
				manager.StorePattern(p)
				log.Printf("pattern detected: type=%s user=%s page=%s severity=%s | %s",
					p.Type, p.UserID, p.Page, p.Severity, p.Explanation)
			}
		}
	}()
}

func main() {
	startConsumer()

	mux := http.NewServeMux()
	mux.HandleFunc("/health", healthHandler)
	mux.HandleFunc("/events", eventHandler)
	mux.HandleFunc("/api/patterns", patternsHandler)
	mux.HandleFunc("/api/stats", statsHandler)
	mux.HandleFunc("/api/users/active", activeUsersHandler)

	server := &http.Server{
		Addr:    ":8080",
		Handler: corsMiddleware(mux), // wraps all routes — CORS applied globally
	}

	log.Println("Server started on :8080")
	log.Fatal(server.ListenAndServe())
}
