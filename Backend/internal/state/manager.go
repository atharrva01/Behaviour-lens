package state

import (
	"sync"
	"sync/atomic"
	"time"
	"behaviourlens/internal/models"
)

// active window used for the "active users" count in metrics.
const activeWindow = 60 * time.Second

const maxPatterns = 500 // maximum patterns kept in memory

// ActiveUser is a lightweight snapshot of a user's current presence.
// Used by GET /api/users/active — avoids exposing full UserState to the API layer.
type ActiveUser struct {
	UserID      string `json:"user_id"`
	CurrentPage string `json:"current_page"`
	LastSeen    int64  `json:"last_seen"` // unix epoch ms
}

type StateManager struct {
	states         map[string]*models.UserState
	mu             sync.RWMutex
	windowDuration time.Duration
	maxEvents      int

	// Pattern store — global across all users, bounded to maxPatterns.
	// Protected by mu (same lock as states; avoids a second lock).
	patterns []models.Pattern

	// Counters use atomic ops so they can be read without holding mu.
	totalEvents   atomic.Int64
	totalPatterns atomic.Int64
}

func NewStateManager(window time.Duration, maxEvents int) *StateManager {
	return &StateManager{
		states:         make(map[string]*models.UserState),
		windowDuration: window,
		maxEvents:      maxEvents,
	}
}

// ProcessEvent updates user state for the incoming event and returns a
// deep-copied snapshot of the updated state. The copy is safe to read
// outside the lock (e.g. in the rule engine) without further synchronization.
func (sm *StateManager) ProcessEvent(e models.Event) models.UserState {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	state, exists := sm.states[e.UserID]
	if !exists {
		state = &models.UserState{
			UserID:          e.UserID,
			SessionStart:    e.Timestamp,
			PageVisitCounts: make(map[string]int),
			TabVisible:      true,
		}
		sm.states[e.UserID] = state
	}

	// Count every event that successfully enters the pipeline.
	sm.totalEvents.Add(1)

	// Update basic tracking fields
	state.Events = append(state.Events, e)
	state.CurrentPage = e.Page
	state.LastSeen = e.Timestamp

	// PageVisitCounts is NOT incremented here.
	// It is rebuilt from the surviving window after trimEvents runs,
	// so it always reflects only the current window — never stale history.

	if e.Action == "tab_hidden" {
		state.TabVisible = false
	}

	if e.Action == "tab_visible" {
		state.TabVisible = true
	}

	if e.Page == "/cart" {
		state.CheckoutDepth = 1
	}
	if e.Page == "/checkout" {
		state.CheckoutDepth = 2
	}
	if e.Page == "/payment" {
		state.CheckoutDepth = 3
	}

	sm.trimEvents(state)

	// Return a deep copy so the rule engine can read state outside this lock.
	// We copy Events and PageVisitCounts because they are reference types;
	// all scalar fields copy by value automatically.
	eventsCopy := make([]models.Event, len(state.Events))
	copy(eventsCopy, state.Events)

	countsCopy := make(map[string]int, len(state.PageVisitCounts))
	for k, v := range state.PageVisitCounts {
		countsCopy[k] = v
	}

	return models.UserState{
		UserID:          state.UserID,
		Events:          eventsCopy,
		CurrentPage:     state.CurrentPage,
		LastSeen:        state.LastSeen,
		SessionStart:    state.SessionStart,
		PageVisitCounts: countsCopy,
		TabVisible:      state.TabVisible,
		CheckoutDepth:   state.CheckoutDepth,
	}
}

func (sm *StateManager) trimEvents(state *models.UserState) {
	cutoff := time.Now().Add(-sm.windowDuration).UnixMilli()

	filtered := state.Events[:0]
	for _, e := range state.Events {
		if e.Timestamp >= cutoff {
			filtered = append(filtered, e)
		}
	}
	state.Events = filtered

	if len(state.Events) > sm.maxEvents {
		state.Events = state.Events[len(state.Events)-sm.maxEvents:]
	}

	// Rebuild PageVisitCounts from only the events still in the window.
	// This ensures loop detection always works on current data, not full history.
	state.PageVisitCounts = make(map[string]int)
	for _, e := range state.Events {
		if e.Action == models.ActionNavigate {
			state.PageVisitCounts[e.Page]++
		}
	}
}

func (sm *StateManager) GetUserEvents(userID string) ([]models.Event, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	state, exists := sm.states[userID]
	if !exists {
		return nil, false
	}

	copied := make([]models.Event, len(state.Events))
	copy(copied, state.Events)

	return copied, true
}

func (sm *StateManager) GetActiveUsers(within time.Duration) int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	cutoff := time.Now().Add(-within).UnixMilli()
	count := 0

	for _, state := range sm.states {
		if state.LastSeen >= cutoff {
			count++
		}
	}

	return count
}

// GetAllActiveUsers returns a snapshot of every user seen within the given duration.
// Each entry contains only the fields the dashboard needs — not the full event history.
func (sm *StateManager) GetAllActiveUsers(within time.Duration) []ActiveUser {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	cutoff := time.Now().Add(-within).UnixMilli()
	result := make([]ActiveUser, 0) // non-nil so JSON encodes as [] not null

	for _, state := range sm.states {
		if state.LastSeen >= cutoff {
			result = append(result, ActiveUser{
				UserID:      state.UserID,
				CurrentPage: state.CurrentPage,
				LastSeen:    state.LastSeen,
			})
		}
	}

	return result
}

// StorePattern appends a detected pattern to the global store.
// If the store exceeds maxPatterns, the oldest entry is evicted.
// Call this after the rule engine emits a pattern.
func (sm *StateManager) StorePattern(p models.Pattern) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.patterns = append(sm.patterns, p)
	if len(sm.patterns) > maxPatterns {
		// Drop the oldest: shift the slice forward by one.
		sm.patterns = sm.patterns[1:]
	}

	sm.totalPatterns.Add(1)
}

// GetPatterns returns up to `limit` patterns, most recent first.
// Passing limit <= 0 returns all stored patterns.
// Returns a defensive copy — callers cannot mutate internal state.
func (sm *StateManager) GetPatterns(limit int) []models.Pattern {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	src := sm.patterns
	if limit > 0 && limit < len(src) {
		// Take the last `limit` entries (they are the most recent).
		src = src[len(src)-limit:]
	}

	// Return a copy in reverse order so index 0 is the most recent pattern.
	result := make([]models.Pattern, len(src))
	for i, p := range src {
		result[len(src)-1-i] = p
	}
	return result
}

// GetMetrics returns a point-in-time SystemMetrics snapshot.
// Atomic counters are read without holding mu; the states map is read under RLock.
func (sm *StateManager) GetMetrics() models.SystemMetrics {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	cutoff := time.Now().Add(-activeWindow).UnixMilli()
	activeCount := 0
	totalUsers := len(sm.states)

	for _, state := range sm.states {
		if state.LastSeen >= cutoff {
			activeCount++
		}
	}

	// Compute abandonment rate: abandonment patterns emitted / total users seen.
	// We scan the live pattern slice here (already under RLock).
	abandonCount := 0
	for _, p := range sm.patterns {
		if p.Type == models.PatternAbandonment {
			abandonCount++
		}
	}

	var abandonRate float64
	if totalUsers > 0 {
		abandonRate = float64(abandonCount) / float64(totalUsers)
	}

	return models.SystemMetrics{
		TotalEvents:      sm.totalEvents.Load(),
		ActiveUsers:      activeCount,
		PatternsDetected: sm.totalPatterns.Load(),
		AbandonmentRate:  abandonRate,
		AsOf:             time.Now().UnixMilli(),
	}
}
