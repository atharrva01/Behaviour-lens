package models

// Pattern type constants — the three behavioral signals BehaviorLens detects.
const (
	PatternHesitation     = "hesitation"
	PatternNavigationLoop = "navigation-loop"
	PatternAbandonment    = "abandonment"
)

// Severity level constants.
const (
	SeverityLow    = "low"
	SeverityMedium = "medium"
	SeverityHigh   = "high"
)

// Pattern is the core output unit of the rule engine.
// One Pattern is emitted each time a behavioral signal is detected for a user.
type Pattern struct {
	PatternID   string `json:"pattern_id"`   // unique identifier, generated at detection time
	UserID      string `json:"user_id"`      // which user triggered this pattern
	Type        string `json:"type"`         // one of the PatternXxx constants above
	Page        string `json:"page"`         // page where the pattern was detected
	DetectedAt  int64  `json:"detected_at"`  // unix epoch milliseconds
	Explanation string `json:"explanation"`  // human-readable description for the dashboard
	Severity    string `json:"severity"`     // one of the SeverityXxx constants above
	Resolved    bool   `json:"resolved"`     // false until user completes the flow
}
