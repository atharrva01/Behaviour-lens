package state

import (
	"testing"
	"time"

	"behaviourlens/internal/models"
)

// ── helpers ───────────────────────────────────────────────────────────────────

func newTestManager() *StateManager {
	// Short window (10s) and small maxEvents for deterministic tests.
	return NewStateManager(10*time.Second, 50)
}

func nowMs() int64 { return time.Now().UnixMilli() }

func makeEvent(userID, action, page string, ts int64) models.Event {
	return models.Event{
		UserID:    userID,
		Action:    action,
		Page:      page,
		Timestamp: ts,
	}
}

// ── state creation ────────────────────────────────────────────────────────────

func TestProcessEventCreatesState(t *testing.T) {
	sm := newTestManager()
	now := nowMs()

	snap := sm.ProcessEvent(makeEvent("u1", "navigate", "/home", now))

	if snap.UserID != "u1" {
		t.Fatalf("expected UserID=u1, got %s", snap.UserID)
	}
	if snap.CurrentPage != "/home" {
		t.Fatalf("expected page=/home, got %s", snap.CurrentPage)
	}
	if len(snap.Events) != 1 {
		t.Fatalf("expected 1 event in snapshot, got %d", len(snap.Events))
	}
	if snap.SessionStart != now {
		t.Fatalf("SessionStart should equal first event timestamp")
	}
}

func TestProcessEventUpdatesCurrentPage(t *testing.T) {
	sm := newTestManager()
	now := nowMs()

	sm.ProcessEvent(makeEvent("u2", "navigate", "/home", now-1000))
	snap := sm.ProcessEvent(makeEvent("u2", "navigate", "/checkout", now))

	if snap.CurrentPage != "/checkout" {
		t.Fatalf("expected CurrentPage=/checkout, got %s", snap.CurrentPage)
	}
}

func TestProcessEventUpdatesLastSeen(t *testing.T) {
	sm := newTestManager()
	now := nowMs()

	sm.ProcessEvent(makeEvent("u3", "navigate", "/home", now-1000))
	snap := sm.ProcessEvent(makeEvent("u3", "click", "/home", now))

	if snap.LastSeen != now {
		t.Fatalf("LastSeen should equal latest event timestamp, got %d", snap.LastSeen)
	}
}

// ── window trimming ───────────────────────────────────────────────────────────

func TestWindowTrimDropsOldEvents(t *testing.T) {
	sm := newTestManager() // window = 10s

	now := nowMs()
	oldTs := now - 20_000 // 20s ago → outside window

	// Submit two old events and one fresh event.
	sm.ProcessEvent(makeEvent("u4", "navigate", "/old1", oldTs))
	sm.ProcessEvent(makeEvent("u4", "navigate", "/old2", oldTs+1000))
	snap := sm.ProcessEvent(makeEvent("u4", "navigate", "/home", now))

	// Only the fresh event should survive.
	if len(snap.Events) != 1 {
		t.Fatalf("expected 1 surviving event after trim, got %d", len(snap.Events))
	}
	if snap.Events[0].Page != "/home" {
		t.Fatalf("surviving event should be /home, got %s", snap.Events[0].Page)
	}
}

func TestWindowTrimKeepsEventsInWindow(t *testing.T) {
	sm := newTestManager() // window = 10s
	now := nowMs()

	for i := 0; i < 5; i++ {
		ts := now - int64(i)*1000 // 0, 1, 2, 3, 4 seconds ago — all within 10s window
		sm.ProcessEvent(makeEvent("u5", "navigate", "/page", ts))
	}

	events, ok := sm.GetUserEvents("u5")
	if !ok {
		t.Fatal("expected events for u5")
	}
	if len(events) != 5 {
		t.Fatalf("expected 5 events in window, got %d", len(events))
	}
}

// ── PageVisitCounts rebuild ───────────────────────────────────────────────────

func TestPageVisitCountsOnlyCountsNavigate(t *testing.T) {
	sm := newTestManager()
	now := nowMs()

	sm.ProcessEvent(makeEvent("u6", "navigate", "/search", now-3000))
	sm.ProcessEvent(makeEvent("u6", "click", "/search", now-2000))       // should NOT count
	sm.ProcessEvent(makeEvent("u6", "scroll", "/search", now-1000))      // should NOT count
	snap := sm.ProcessEvent(makeEvent("u6", "navigate", "/search", now)) // should count

	if snap.PageVisitCounts["/search"] != 2 {
		t.Fatalf("expected 2 navigate events on /search, got %d", snap.PageVisitCounts["/search"])
	}
}

func TestPageVisitCountsDroppedWithOldEvents(t *testing.T) {
	sm := newTestManager() // window = 10s
	now := nowMs()

	// Old navigate — will be trimmed.
	sm.ProcessEvent(makeEvent("u7", "navigate", "/old", now-20_000))
	// Fresh navigate.
	snap := sm.ProcessEvent(makeEvent("u7", "navigate", "/fresh", now))

	if _, exists := snap.PageVisitCounts["/old"]; exists {
		t.Fatal("/old should have been trimmed from PageVisitCounts")
	}
	if snap.PageVisitCounts["/fresh"] != 1 {
		t.Fatalf("expected 1 for /fresh, got %d", snap.PageVisitCounts["/fresh"])
	}
}

// ── checkout depth ────────────────────────────────────────────────────────────

func TestCheckoutDepthProgression(t *testing.T) {
	sm := newTestManager()
	now := nowMs()

	sm.ProcessEvent(makeEvent("u8", "navigate", "/cart", now-2000))
	sm.ProcessEvent(makeEvent("u8", "navigate", "/checkout", now-1000))
	snap := sm.ProcessEvent(makeEvent("u8", "navigate", "/payment", now))

	if snap.CheckoutDepth != 3 {
		t.Fatalf("expected CheckoutDepth=3, got %d", snap.CheckoutDepth)
	}
}

func TestTabVisibilityTransitions(t *testing.T) {
	sm := newTestManager()
	now := nowMs()

	sm.ProcessEvent(makeEvent("u8b", models.ActionNavigate, "/home", now-2000))
	hidden := sm.ProcessEvent(makeEvent("u8b", models.ActionTabHidden, "/home", now-1000))
	if hidden.TabVisible {
		t.Fatal("expected tab to be hidden after tab_hidden event")
	}

	visible := sm.ProcessEvent(makeEvent("u8b", models.ActionTabVisible, "/home", now))
	if !visible.TabVisible {
		t.Fatal("expected tab to be visible after tab_visible event")
	}
}

// ── pattern store ─────────────────────────────────────────────────────────────

func makePattern(patternType, userID, page string, detectedAt int64) models.Pattern {
	return models.Pattern{
		PatternID:   userID + "_" + patternType,
		UserID:      userID,
		Type:        patternType,
		Page:        page,
		DetectedAt:  detectedAt,
		Severity:    models.SeverityLow,
		Explanation: "test",
	}
}

func TestGetPatternsMostRecentFirst(t *testing.T) {
	sm := newTestManager()
	now := nowMs()

	sm.StorePattern(makePattern(models.PatternHesitation, "u9", "/checkout", now-3000))
	sm.StorePattern(makePattern(models.PatternNavigationLoop, "u9", "/search", now-2000))
	sm.StorePattern(makePattern(models.PatternAbandonment, "u9", "/home", now-1000))

	patterns := sm.GetPatterns(0)
	if len(patterns) != 3 {
		t.Fatalf("expected 3 patterns, got %d", len(patterns))
	}
	// Index 0 should be the most recent (highest DetectedAt).
	if patterns[0].DetectedAt < patterns[1].DetectedAt {
		t.Fatal("patterns should be returned most-recent-first")
	}
	if patterns[1].DetectedAt < patterns[2].DetectedAt {
		t.Fatal("patterns should be returned most-recent-first")
	}
}

func TestGetPatternsLimit(t *testing.T) {
	sm := newTestManager()
	now := nowMs()

	for i := 0; i < 10; i++ {
		sm.StorePattern(makePattern(models.PatternHesitation, "u10", "/page",
			now-int64(i)*1000))
	}

	patterns := sm.GetPatterns(3)
	if len(patterns) != 3 {
		t.Fatalf("expected 3 patterns with limit=3, got %d", len(patterns))
	}
}

// ── metrics ───────────────────────────────────────────────────────────────────

func TestGetMetricsTotalEventsCount(t *testing.T) {
	sm := newTestManager()
	now := nowMs()

	for i := 0; i < 5; i++ {
		sm.ProcessEvent(makeEvent("u11", "navigate", "/home", now-int64(i)*500))
	}

	metrics := sm.GetMetrics()
	if metrics.TotalEvents != 5 {
		t.Fatalf("expected TotalEvents=5, got %d", metrics.TotalEvents)
	}
}

func TestGetMetricsPatternsDetectedCount(t *testing.T) {
	sm := newTestManager()
	now := nowMs()

	sm.StorePattern(makePattern(models.PatternHesitation, "u12", "/p", now))
	sm.StorePattern(makePattern(models.PatternAbandonment, "u12", "/p", now))

	metrics := sm.GetMetrics()
	if metrics.PatternsDetected != 2 {
		t.Fatalf("expected PatternsDetected=2, got %d", metrics.PatternsDetected)
	}
}

// ── active users ──────────────────────────────────────────────────────────────

func TestGetAllActiveUsersWithinWindow(t *testing.T) {
	sm := newTestManager()
	now := nowMs()

	sm.ProcessEvent(makeEvent("active_user", "navigate", "/home", now-5_000))  // 5s ago → active
	sm.ProcessEvent(makeEvent("stale_user", "navigate", "/home", now-120_000)) // 2m ago → stale

	active := sm.GetAllActiveUsers(30 * time.Second)

	for _, u := range active {
		if u.UserID == "stale_user" {
			t.Fatal("stale_user should not appear in active users list")
		}
	}

	found := false
	for _, u := range active {
		if u.UserID == "active_user" {
			found = true
		}
	}
	if !found {
		t.Fatal("active_user should appear in active users list")
	}
}

// ── snapshot isolation ────────────────────────────────────────────────────────

func TestSnapshotIsolation(t *testing.T) {
	sm := newTestManager()
	now := nowMs()

	snap := sm.ProcessEvent(makeEvent("u13", "navigate", "/home", now))

	// Mutate the snapshot's Events slice — should not affect internal state.
	if len(snap.Events) > 0 {
		snap.Events[0].Page = "mutated"
	}

	events, _ := sm.GetUserEvents("u13")
	if len(events) > 0 && events[0].Page == "mutated" {
		t.Fatal("snapshot mutation should not affect internal state")
	}
}
