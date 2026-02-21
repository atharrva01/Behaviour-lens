package models

// SystemMetrics is a point-in-time snapshot of backend health and activity.
// Served by GET /api/stats and consumed by the dashboard stats bar.
type SystemMetrics struct {
	TotalEvents       int64   `json:"total_events"`       // cumulative events processed since startup
	ActiveUsers       int     `json:"active_users"`       // users seen in the last N seconds
	PatternsDetected  int64   `json:"patterns_detected"`  // cumulative patterns emitted since startup
	AbandonmentRate   float64 `json:"abandonment_rate"`   // abandonment patterns / total users seen
	AsOf              int64   `json:"as_of"`              // unix epoch ms when this snapshot was taken
}
