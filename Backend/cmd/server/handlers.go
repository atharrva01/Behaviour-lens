package main

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ── shared helpers ────────────────────────────────────────────────────────────

// writeJSON sets Content-Type, writes the status code, and encodes data as JSON.
// All API success responses go through this — no ad-hoc header setting elsewhere.
func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// ── CORS middleware ───────────────────────────────────────────────────────────

// corsMiddleware wraps any handler and adds the headers the browser needs to
// allow cross-origin requests from the React frontend (different port/origin).
//
// Access-Control-Allow-Origin: * is intentional for this MVP — the system has
// no authentication, so restricting the origin provides no security benefit.
// In a production system with auth tokens you would restrict this to the
// specific frontend origin.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		// Preflight request — the browser sends OPTIONS before the real request
		// to check permissions. Return 200 immediately; do not call next.
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// ── GET /api/patterns ─────────────────────────────────────────────────────────

// patternsHandler returns detected patterns, most recent first.
// Optional query params:
//   - ?limit=N   (default 50, must be > 0)
//   - ?severity=low|medium|high  (filter by severity)
func patternsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	limit := 50
	if s := r.URL.Query().Get("limit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			limit = n
		}
	}

	patterns := manager.GetPatterns(limit)

	// Optional severity filter — applied after retrieval so limit still applies.
	if severity := strings.TrimSpace(r.URL.Query().Get("severity")); severity != "" {
		filtered := patterns[:0]
		for _, p := range patterns {
			if p.Severity == severity {
				filtered = append(filtered, p)
			}
		}
		patterns = filtered
	}

	writeJSON(w, http.StatusOK, patterns)
}

// ── PATCH /api/patterns/{id}/resolve ─────────────────────────────────────────

// resolvePatternHandler marks a pattern as resolved and re-broadcasts it via SSE
// so the dashboard can update the card in real time without a full reload.
func resolvePatternHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Path: /api/patterns/{id}/resolve
	path := strings.TrimPrefix(r.URL.Path, "/api/patterns/")
	path = strings.TrimSuffix(path, "/resolve")
	patternID := strings.TrimSpace(path)

	if patternID == "" {
		http.Error(w, "pattern_id missing from path", http.StatusBadRequest)
		return
	}

	resolved, found := manager.ResolvePattern(patternID)
	if !found {
		http.Error(w, "Pattern not found", http.StatusNotFound)
		return
	}

	// Broadcast the updated (resolved) pattern so live clients see the change.
	hub.BroadcastPattern(resolved)

	writeJSON(w, http.StatusOK, resolved)
}

// ── GET /api/stats ────────────────────────────────────────────────────────────

// statsHandler returns a SystemMetrics snapshot for the dashboard stats bar.
func statsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	writeJSON(w, http.StatusOK, manager.GetMetrics())
}

// ── GET /api/users/active ─────────────────────────────────────────────────────

// activeUsersHandler returns users seen within the last 60 seconds.
// Optional query param: ?within=N (seconds, default 60).
func activeUsersHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	within := 60 * time.Second
	if s := r.URL.Query().Get("within"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			within = time.Duration(n) * time.Second
		}
	}

	// GetAllActiveUsers uses make() internally — always returns [] not null in JSON.
	writeJSON(w, http.StatusOK, manager.GetAllActiveUsers(within))
}
