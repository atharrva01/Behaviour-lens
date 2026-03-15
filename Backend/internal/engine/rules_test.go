package engine

import (
	"testing"
	"time"

	"behaviourlens/internal/models"
)

// now returns the current epoch milliseconds.
func nowMs() int64 { return time.Now().UnixMilli() }

// makeEvent is a test helper to construct an Event with the given fields.
func makeEvent(userID, action, page string, ts int64, meta map[string]string) models.Event {
	return models.Event{
		UserID:    userID,
		Action:    action,
		Page:      page,
		Timestamp: ts,
		Metadata:  meta,
	}
}

// ── hesitation ────────────────────────────────────────────────────────────────

func TestHesitationLowSeverity(t *testing.T) {
	re := NewRuleEngine()
	now := nowMs()

	state := models.UserState{
		UserID:          "u1",
		CurrentPage:     "/checkout",
		PageVisitCounts: map[string]int{},
		Events: []models.Event{
			makeEvent("u1", models.ActionIdle, "/checkout", now-100,
				map[string]string{"duration_ms": "12000"}), // 12s → low
		},
	}

	patterns := re.Evaluate(state)
	if len(patterns) == 0 {
		t.Fatal("expected hesitation pattern, got none")
	}
	p := patterns[0]
	if p.Type != models.PatternHesitation {
		t.Fatalf("expected type=%s, got %s", models.PatternHesitation, p.Type)
	}
	if p.Severity != models.SeverityLow {
		t.Fatalf("expected severity=low, got %s", p.Severity)
	}
	if p.Page != "/checkout" {
		t.Fatalf("expected page=/checkout, got %s", p.Page)
	}
}

func TestHesitationMediumSeverity(t *testing.T) {
	re := NewRuleEngine()
	now := nowMs()
	state := models.UserState{
		UserID:          "u2",
		CurrentPage:     "/payment",
		PageVisitCounts: map[string]int{},
		Events: []models.Event{
			makeEvent("u2", models.ActionIdle, "/payment", now-100,
				map[string]string{"duration_ms": "25000"}), // 25s → medium
		},
	}
	patterns := re.Evaluate(state)
	if len(patterns) == 0 {
		t.Fatal("expected hesitation pattern")
	}
	if patterns[0].Severity != models.SeverityMedium {
		t.Fatalf("expected medium, got %s", patterns[0].Severity)
	}
}

func TestHesitationHighSeverity(t *testing.T) {
	re := NewRuleEngine()
	now := nowMs()
	state := models.UserState{
		UserID:          "u3",
		CurrentPage:     "/checkout",
		PageVisitCounts: map[string]int{},
		Events: []models.Event{
			makeEvent("u3", models.ActionIdle, "/checkout", now-100,
				map[string]string{"duration_ms": "45000"}), // 45s → high
		},
	}
	patterns := re.Evaluate(state)
	if len(patterns) == 0 {
		t.Fatal("expected hesitation pattern")
	}
	if patterns[0].Severity != models.SeverityHigh {
		t.Fatalf("expected high, got %s", patterns[0].Severity)
	}
}

func TestHesitationBelowThreshold(t *testing.T) {
	re := NewRuleEngine()
	now := nowMs()
	state := models.UserState{
		UserID:          "u4",
		CurrentPage:     "/home",
		PageVisitCounts: map[string]int{},
		Events: []models.Event{
			makeEvent("u4", models.ActionIdle, "/home", now-100,
				map[string]string{"duration_ms": "5000"}), // 5s → below threshold
		},
	}
	patterns := re.Evaluate(state)
	for _, p := range patterns {
		if p.Type == models.PatternHesitation {
			t.Fatal("unexpected hesitation for idle < 10s")
		}
	}
}

func TestHesitationCooldown(t *testing.T) {
	re := NewRuleEngine()
	now := nowMs()

	state := models.UserState{
		UserID:          "u5",
		CurrentPage:     "/checkout",
		PageVisitCounts: map[string]int{},
		Events: []models.Event{
			makeEvent("u5", models.ActionIdle, "/checkout", now-100,
				map[string]string{"duration_ms": "15000"}),
		},
	}

	// First call — should detect.
	p1 := re.Evaluate(state)
	if len(p1) == 0 {
		t.Fatal("first evaluation: expected pattern")
	}

	// Same state, immediate second call — should be on cooldown.
	p2 := re.Evaluate(state)
	for _, p := range p2 {
		if p.Type == models.PatternHesitation {
			t.Fatal("second evaluation: should be on cooldown")
		}
	}
}

func TestHesitationWrongPage(t *testing.T) {
	// Idle happened on /cart but user is now on /checkout — should NOT fire.
	re := NewRuleEngine()
	now := nowMs()
	state := models.UserState{
		UserID:          "u6",
		CurrentPage:     "/checkout",
		PageVisitCounts: map[string]int{},
		Events: []models.Event{
			makeEvent("u6", models.ActionIdle, "/cart", now-100,
				map[string]string{"duration_ms": "15000"}),
		},
	}
	patterns := re.Evaluate(state)
	for _, p := range patterns {
		if p.Type == models.PatternHesitation {
			t.Fatal("should not fire hesitation for idle on a different page")
		}
	}
}

// ── navigation loop ───────────────────────────────────────────────────────────

func TestNavigationLoopDetected(t *testing.T) {
	re := NewRuleEngine()
	now := nowMs()

	events := []models.Event{
		makeEvent("u7", models.ActionNavigate, "/search", now-5000, nil),
		makeEvent("u7", models.ActionNavigate, "/products", now-4000, nil),
		makeEvent("u7", models.ActionNavigate, "/search", now-3000, nil),
		makeEvent("u7", models.ActionNavigate, "/search", now-2000, nil),
	}

	state := models.UserState{
		UserID:      "u7",
		CurrentPage: "/search",
		PageVisitCounts: map[string]int{
			"/search":   3, // meets loopMinVisits threshold
			"/products": 1,
		},
		Events: events,
	}

	patterns := re.Evaluate(state)
	found := false
	for _, p := range patterns {
		if p.Type == models.PatternNavigationLoop {
			found = true
			if p.Page != "/search" {
				t.Fatalf("expected loop page=/search, got %s", p.Page)
			}
		}
	}
	if !found {
		t.Fatal("expected navigation-loop pattern")
	}
}

func TestNavigationLoopBelowThreshold(t *testing.T) {
	re := NewRuleEngine()
	now := nowMs()

	state := models.UserState{
		UserID:      "u8",
		CurrentPage: "/search",
		PageVisitCounts: map[string]int{
			"/search": 2, // below loopMinVisits=3
		},
		Events: []models.Event{
			makeEvent("u8", models.ActionNavigate, "/search", now-2000, nil),
			makeEvent("u8", models.ActionNavigate, "/search", now-1000, nil),
		},
	}

	patterns := re.Evaluate(state)
	for _, p := range patterns {
		if p.Type == models.PatternNavigationLoop {
			t.Fatal("should not fire: visit count < loopMinVisits")
		}
	}
}

// ── abandonment ───────────────────────────────────────────────────────────────

func TestAbandonmentExplicit(t *testing.T) {
	re := NewRuleEngine()
	now := nowMs()

	state := models.UserState{
		UserID:        "u9",
		CurrentPage:   "/checkout",
		CheckoutDepth: 2,
		PageVisitCounts: map[string]int{
			"/checkout": 1,
		},
		Events: []models.Event{
			makeEvent("u9", models.ActionNavigate, "/checkout", now-3000, nil),
			makeEvent("u9", models.ActionAbandon, "/checkout", now-100, nil),
		},
	}

	patterns := re.Evaluate(state)
	found := false
	for _, p := range patterns {
		if p.Type == models.PatternAbandonment {
			found = true
			if p.Severity != models.SeverityHigh {
				t.Fatalf("abandonment should always be high severity, got %s", p.Severity)
			}
		}
	}
	if !found {
		t.Fatal("expected abandonment pattern")
	}
}

func TestAbandonmentNavigateAway(t *testing.T) {
	re := NewRuleEngine()
	now := nowMs()

	state := models.UserState{
		UserID:        "u10",
		CurrentPage:   "/home",
		CheckoutDepth: 2,
		PageVisitCounts: map[string]int{
			"/checkout": 1,
		},
		Events: []models.Event{
			makeEvent("u10", models.ActionNavigate, "/checkout", now-3000, nil),
			makeEvent("u10", models.ActionNavigate, "/home", now-100, nil), // navigate away
		},
	}

	patterns := re.Evaluate(state)
	found := false
	for _, p := range patterns {
		if p.Type == models.PatternAbandonment {
			found = true
		}
	}
	if !found {
		t.Fatal("expected abandonment on navigate-away from checkout")
	}
}

func TestAbandonmentNotFiredAfterPurchase(t *testing.T) {
	re := NewRuleEngine()
	now := nowMs()

	state := models.UserState{
		UserID:        "u11",
		CurrentPage:   "/confirm",
		CheckoutDepth: 3,
		PageVisitCounts: map[string]int{
			"/payment": 1,
		},
		Events: []models.Event{
			makeEvent("u11", models.ActionNavigate, "/payment", now-2000, nil),
			// completion event in the window
			makeEvent("u11", models.ActionPurchase, "/confirm", now-500, nil),
			makeEvent("u11", models.ActionNavigate, "/home", now-100, nil),
		},
	}

	patterns := re.Evaluate(state)
	for _, p := range patterns {
		if p.Type == models.PatternAbandonment {
			t.Fatal("should not fire abandonment after purchase event")
		}
	}
}

func TestAbandonmentNotFiredAfterConfirmPageNavigation(t *testing.T) {
	re := NewRuleEngine()
	now := nowMs()

	state := models.UserState{
		UserID:        "u11b",
		CurrentPage:   "/home",
		CheckoutDepth: 3,
		PageVisitCounts: map[string]int{
			"/payment": 1,
		},
		Events: []models.Event{
			makeEvent("u11b", models.ActionNavigate, "/payment", now-2000, nil),
			makeEvent("u11b", models.ActionNavigate, "/confirm", now-500, nil),
			makeEvent("u11b", models.ActionNavigate, "/home", now-100, nil),
		},
	}

	patterns := re.Evaluate(state)
	for _, p := range patterns {
		if p.Type == models.PatternAbandonment {
			t.Fatal("should not fire abandonment after navigating to confirm page")
		}
	}
}

func TestAbandonmentRequiresCheckoutDepth(t *testing.T) {
	re := NewRuleEngine()
	now := nowMs()

	state := models.UserState{
		UserID:          "u12",
		CurrentPage:     "/home",
		CheckoutDepth:   1, // only at /cart — depth < 2
		PageVisitCounts: map[string]int{"/cart": 1},
		Events: []models.Event{
			makeEvent("u12", models.ActionNavigate, "/cart", now-1000, nil),
			makeEvent("u12", models.ActionNavigate, "/home", now-100, nil),
		},
	}

	patterns := re.Evaluate(state)
	for _, p := range patterns {
		if p.Type == models.PatternAbandonment {
			t.Fatal("should not fire abandonment with depth < 2")
		}
	}
}

// ── no false positives ────────────────────────────────────────────────────────

func TestNormalBrowsingNoPatterns(t *testing.T) {
	re := NewRuleEngine()
	now := nowMs()

	state := models.UserState{
		UserID:          "u13",
		CurrentPage:     "/products",
		CheckoutDepth:   0,
		PageVisitCounts: map[string]int{"/products": 1, "/home": 1},
		Events: []models.Event{
			makeEvent("u13", models.ActionNavigate, "/home", now-5000, nil),
			makeEvent("u13", models.ActionScroll, "/home", now-4000, nil),
			makeEvent("u13", models.ActionNavigate, "/products", now-3000, nil),
			makeEvent("u13", models.ActionClick, "/products", now-2000, nil),
		},
	}

	patterns := re.Evaluate(state)
	if len(patterns) != 0 {
		t.Fatalf("expected 0 patterns for normal browsing, got %d: %+v", len(patterns), patterns)
	}
}

// ── explain engine ────────────────────────────────────────────────────────────

func TestExplainHesitationContainsSeconds(t *testing.T) {
	now := nowMs()
	p := models.Pattern{
		Type: models.PatternHesitation,
		Page: "/checkout",
	}
	state := models.UserState{
		UserID:      "u14",
		CurrentPage: "/checkout",
		Events: []models.Event{
			makeEvent("u14", models.ActionIdle, "/checkout", now-100,
				map[string]string{"duration_ms": "15000"}),
		},
	}
	explanation := ExplainPattern(p, state)
	if explanation == "" {
		t.Fatal("explanation should not be empty")
	}
	// Should mention the page.
	if explanation == "Behavioral pattern detected." {
		t.Fatal("expected specific explanation, got generic fallback")
	}
}

func TestExplainNavigationLoop(t *testing.T) {
	now := nowMs()
	p := models.Pattern{
		Type: models.PatternNavigationLoop,
		Page: "/search",
	}
	state := models.UserState{
		UserID: "u15",
		PageVisitCounts: map[string]int{
			"/search": 4,
		},
		Events: []models.Event{
			makeEvent("u15", models.ActionNavigate, "/search", now-60000, nil),
			makeEvent("u15", models.ActionNavigate, "/search", now-100, nil),
		},
	}
	explanation := ExplainPattern(p, state)
	if explanation == "" {
		t.Fatal("explanation should not be empty")
	}
}

func TestExplainAbandonment(t *testing.T) {
	p := models.Pattern{
		Type: models.PatternAbandonment,
		Page: "/home",
	}
	state := models.UserState{
		UserID:        "u16",
		CheckoutDepth: 2,
	}
	explanation := ExplainPattern(p, state)
	if explanation == "" {
		t.Fatal("explanation should not be empty")
	}
}
