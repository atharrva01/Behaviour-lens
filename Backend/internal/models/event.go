package models

// Valid action types for an incoming Event.
// The ingestion handler validates against these; the rule engine branches on them.
const (
	ActionClick    = "click"
	ActionScroll   = "scroll"
	ActionIdle     = "idle"
	ActionNavigate = "navigate"
	ActionAbandon  = "abandon"
)

// ValidActions is the set of accepted action strings.
// Used for O(1) validation in the ingestion handler.
var ValidActions = map[string]bool{
	ActionClick:    true,
	ActionScroll:   true,
	ActionIdle:     true,
	ActionNavigate: true,
	ActionAbandon:  true,
}

type Event struct {
	UserID    string            `json:"user_id"`
	Action    string            `json:"action"`
	Page      string            `json:"page"`
	Timestamp int64             `json:"timestamp"`
	Metadata  map[string]string `json:"metadata"`
}
