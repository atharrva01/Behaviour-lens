package engine

import (
	"fmt"

	"behaviourlens/internal/models"
)

// ExplainPattern generates a plain-English explanation for a detected pattern.
// It is called after rule detection, before the pattern is stored.
//
// Design: explanations are rule-based in MVP — deterministic and auditable.
// The function receives both the pattern (what was detected) and the user state
// snapshot (the evidence), so each explanation can cite real numbers.
func ExplainPattern(p models.Pattern, state models.UserState) string {
	switch p.Type {
	case models.PatternHesitation:
		return explainHesitation(p, state)
	case models.PatternNavigationLoop:
		return explainNavigationLoop(p, state)
	case models.PatternAbandonment:
		return explainAbandonment(p, state)
	default:
		return "Behavioral pattern detected."
	}
}

// ── hesitation ────────────────────────────────────────────────────────────────

func explainHesitation(p models.Pattern, state models.UserState) string {
	// Find the longest idle duration on the pattern's page.
	// Reuses idleDuration() from rules.go — same package, no duplication.
	maxDurMs := int64(0)
	for i, e := range state.Events {
		if e.Action != models.ActionIdle || e.Page != p.Page {
			continue
		}
		if dur := idleDuration(e, state.Events, i); dur > maxDurMs {
			maxDurMs = dur
		}
	}

	secs := maxDurMs / 1000
	if secs == 0 {
		return fmt.Sprintf("User paused on %s without taking action.", p.Page)
	}
	return fmt.Sprintf(
		"User paused for %ds on %s without taking action. This may indicate confusion or comparison hesitation.",
		secs, p.Page,
	)
}

// ── navigation loop ───────────────────────────────────────────────────────────

func explainNavigationLoop(p models.Pattern, state models.UserState) string {
	count := state.PageVisitCounts[p.Page]
	if count == 0 {
		// Fallback: count was trimmed out since detection. Use generic phrasing.
		return fmt.Sprintf("User navigated to %s multiple times, suggesting difficulty progressing.", p.Page)
	}

	// Compute the actual observed time span from the event window.
	spanDesc := windowSpanDescription(state.Events)

	return fmt.Sprintf(
		"User navigated to %s %d times %s, suggesting they are stuck or cannot find what they need.",
		p.Page, count, spanDesc,
	)
}

// windowSpanDescription returns a human-readable description of the time span
// covered by the current event window (e.g. "in the last 3 minutes").
func windowSpanDescription(events []models.Event) string {
	if len(events) < 2 {
		return "recently"
	}
	spanMs := events[len(events)-1].Timestamp - events[0].Timestamp
	spanMins := spanMs / 1000 / 60
	switch {
	case spanMins <= 0:
		return "in the last minute"
	case spanMins == 1:
		return "in the last 1 minute"
	default:
		return fmt.Sprintf("in the last %d minutes", spanMins)
	}
}

// ── abandonment ───────────────────────────────────────────────────────────────

func explainAbandonment(p models.Pattern, state models.UserState) string {
	stage := checkoutStage(state.CheckoutDepth)
	return fmt.Sprintf(
		"User reached %s but navigated away without completing the purchase. Checkout flow was abandoned at the %s stage.",
		p.Page, stage,
	)
}

// checkoutStage translates CheckoutDepth into a readable stage name.
func checkoutStage(depth int) string {
	switch depth {
	case 1:
		return "cart"
	case 2:
		return "checkout"
	case 3:
		return "payment"
	default:
		return "checkout"
	}
}
