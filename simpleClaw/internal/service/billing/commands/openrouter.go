package commands

import "time"

type OpenRouterUsageEvent struct {
	TraceID     string
	SpanID      string
	APIKeyName  string
	Model       string
	TotalCost   string
	OccurredAt  time.Time
	Description string
}
