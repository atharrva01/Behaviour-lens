package engine

import (
	"fmt"
	"strconv"
	"time"

	"behaviourlens/internal/models"
)

// Detection thresholds — all durations in milliseconds.
const (
	hesitationLowMs    = 10_000 // 10s  → low severity
	hesitationMediumMs = 20_000 // 20s  → medium severity
	hesitationHighMs   = 40_000 // 40s  → high severity

	loopMinVisits = 3 // minimum page visits within window to trigger loop

	cooldownMs = 2 * 60 * 1000 // 2 minutes between identical pattern emissions
)

// RuleEngine evaluates behavioral rules against a user state snapshot.
// It is called synchronously inside the consumer goroutine after every
// ProcessEvent — it must be fast and must not acquire any locks of its own.
type RuleEngine struct {
	// lastDetected tracks the timestamp of the last emission per user+type+page.
	// Key: "userID:patternType:page". Single-goroutine access — no mutex needed.
	lastDetected map[string]int64
}

// NewRuleEngine constructs a ready-to-use RuleEngine.
func NewRuleEngine() *RuleEngine {
	return &RuleEngine{
		lastDetected: make(map[string]int64),
	}
}

// Evaluate runs all three detection rules against the given state snapshot.
// Returns all patterns detected on this call (may be empty).
// Explanations are NOT set here — that is Phase 4 (Explain Engine).
func (re *RuleEngine) Evaluate(state models.UserState) []models.Pattern {
	now := time.Now().UnixMilli()
	var detected []models.Pattern

	if p, ok := re.detectHesitation(state, now); ok {
		detected = append(detected, p)
	}
	if p, ok := re.detectNavigationLoop(state, now); ok {
		detected = append(detected, p)
	}
	if p, ok := re.detectAbandonment(state, now); ok {
		detected = append(detected, p)
	}

	return detected
}

// ── helpers ───────────────────────────────────────────────────────────────────

func (re *RuleEngine) cooldownKey(userID, patternType, page string) string {
	return fmt.Sprintf("%s:%s:%s", userID, patternType, page)
}

func (re *RuleEngine) isOnCooldown(userID, patternType, page string, now int64) bool {
	last, exists := re.lastDetected[re.cooldownKey(userID, patternType, page)]
	return exists && (now-last) < cooldownMs
}

func (re *RuleEngine) markDetected(userID, patternType, page string, now int64) {
	re.lastDetected[re.cooldownKey(userID, patternType, page)] = now
}

func patternID(userID, patternType string, now int64) string {
	// Simple unique ID: no external UUID package needed.
	// Collisions are impossible within a single user's timeline.
	return fmt.Sprintf("%s_%s_%d", userID, patternType, now)
}

// ── hesitation ────────────────────────────────────────────────────────────────

// detectHesitation looks for idle events on the current page whose duration
// exceeds the minimum threshold. Fires at most once per cooldown window.
func (re *RuleEngine) detectHesitation(state models.UserState, now int64) (models.Pattern, bool) {
	if re.isOnCooldown(state.UserID, models.PatternHesitation, state.CurrentPage, now) {
		return models.Pattern{}, false
	}

	for i, e := range state.Events {
		// Only consider idle events that happened on the current page.
		if e.Action != models.ActionIdle || e.Page != state.CurrentPage {
			continue
		}

		durationMs := idleDuration(e, state.Events, i)
		if durationMs < hesitationLowMs {
			continue
		}

		re.markDetected(state.UserID, models.PatternHesitation, state.CurrentPage, now)
		return models.Pattern{
			PatternID:  patternID(state.UserID, models.PatternHesitation, now),
			UserID:     state.UserID,
			Type:       models.PatternHesitation,
			Page:       state.CurrentPage,
			DetectedAt: now,
			Severity:   hesitationSeverity(durationMs),
			Resolved:   false,
		}, true
	}

	return models.Pattern{}, false
}

// idleDuration resolves how long an idle event lasted.
// Priority: metadata["duration_ms"] → timestamp delta from previous event → 0.
func idleDuration(e models.Event, events []models.Event, idx int) int64 {
	if durStr, ok := e.Metadata["duration_ms"]; ok {
		if dur, err := strconv.ParseInt(durStr, 10, 64); err == nil && dur > 0 {
			return dur
		}
	}
	// Fall back: gap between this event and the one before it.
	if idx > 0 {
		delta := e.Timestamp - events[idx-1].Timestamp
		if delta > 0 {
			return delta
		}
	}
	return 0
}

func hesitationSeverity(durationMs int64) string {
	switch {
	case durationMs >= hesitationHighMs:
		return models.SeverityHigh
	case durationMs >= hesitationMediumMs:
		return models.SeverityMedium
	default:
		return models.SeverityLow
	}
}

// ── navigation loop ───────────────────────────────────────────────────────────

// detectNavigationLoop checks if any single page has been visited enough times
// within the current window to indicate the user is stuck in a loop.
func (re *RuleEngine) detectNavigationLoop(state models.UserState, now int64) (models.Pattern, bool) {
	// Find the page with the highest windowed visit count.
	maxPage, maxCount := "", 0
	for page, count := range state.PageVisitCounts {
		if count > maxCount {
			maxPage, maxCount = page, count
		}
	}

	if maxCount < loopMinVisits {
		return models.Pattern{}, false
	}

	if re.isOnCooldown(state.UserID, models.PatternNavigationLoop, maxPage, now) {
		return models.Pattern{}, false
	}

	re.markDetected(state.UserID, models.PatternNavigationLoop, maxPage, now)
	return models.Pattern{
		PatternID:  patternID(state.UserID, models.PatternNavigationLoop, now),
		UserID:     state.UserID,
		Type:       models.PatternNavigationLoop,
		Page:       maxPage,
		DetectedAt: now,
		Severity:   loopSeverity(maxCount),
		Resolved:   false,
	}, true
}

func loopSeverity(count int) string {
	switch {
	case count >= 5:
		return models.SeverityHigh
	case count >= 4:
		return models.SeverityMedium
	default:
		return models.SeverityLow
	}
}

// ── abandonment ───────────────────────────────────────────────────────────────

// detectAbandonment fires when a user who was deep in the checkout flow
// navigates away or sends an explicit abandon action without completing.
func (re *RuleEngine) detectAbandonment(state models.UserState, now int64) (models.Pattern, bool) {
	if len(state.Events) == 0 {
		return models.Pattern{}, false
	}

	// Require the user to have reached at least /checkout or /payment.
	if state.CheckoutDepth < 2 {
		return models.Pattern{}, false
	}

	// Require at least one checkout-flow event within the current window.
	// Guards against stale CheckoutDepth from a long-past session.
	if !hasRecentCheckoutEvent(state.Events) {
		return models.Pattern{}, false
	}

	// If the user has already completed the flow, do not fire.
	if hasCompletionEvent(state.Events) {
		return models.Pattern{}, false
	}

	// Fire if the triggering event is an explicit abandon,
	// or if the user navigated away from the checkout flow entirely.
	lastEvent := state.Events[len(state.Events)-1]
	isExplicitAbandon := lastEvent.Action == models.ActionAbandon
	isNavigatedAway := lastEvent.Action == models.ActionNavigate && !isCheckoutPage(lastEvent.Page)

	if !isExplicitAbandon && !isNavigatedAway {
		return models.Pattern{}, false
	}

	if re.isOnCooldown(state.UserID, models.PatternAbandonment, lastEvent.Page, now) {
		return models.Pattern{}, false
	}

	re.markDetected(state.UserID, models.PatternAbandonment, lastEvent.Page, now)
	return models.Pattern{
		PatternID:  patternID(state.UserID, models.PatternAbandonment, now),
		UserID:     state.UserID,
		Type:       models.PatternAbandonment,
		Page:       lastEvent.Page,
		DetectedAt: now,
		Severity:   models.SeverityHigh,
		Resolved:   false,
	}, true
}

func isCheckoutPage(page string) bool {
	return page == "/cart" || page == "/checkout" || page == "/payment"
}

// hasRecentCheckoutEvent returns true if at least one event in the window
// occurred on a checkout-flow page.
func hasRecentCheckoutEvent(events []models.Event) bool {
	for _, e := range events {
		if isCheckoutPage(e.Page) {
			return true
		}
	}
	return false
}

// hasCompletionEvent returns true if the user already completed the flow.
// "confirm" and "purchase" are the terminal success actions.
func hasCompletionEvent(events []models.Event) bool {
	for _, e := range events {
		if e.Action == models.ActionConfirm || e.Action == models.ActionPurchase {
			return true
		}
		if e.Action == models.ActionNavigate && e.Page == "/confirm" {
			return true
		}
	}
	return false
}
