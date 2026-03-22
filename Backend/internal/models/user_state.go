package models

type UserState struct {
	UserID string

	Events []Event

	CurrentPage   string
	LastSeen      int64
	SessionStart  int64
	PageVisitCounts map[string]int

	TabVisible    bool
	CheckoutDepth int
}
