package profiles

import "time"

// Maintenance contains user-authored shortcuts and an opt-in refresh policy.
type AirportMaintenance struct {
	RefreshHours int               `json:"refresh_hours"`
	NextRefresh  time.Time         `json:"next_refresh,omitempty"`
	LastAttempt  time.Time         `json:"last_attempt,omitempty"`
	LastResult   string            `json:"last_result,omitempty"`
	Links        []MaintenanceLink `json:"links"`
}

type MaintenanceLink struct {
	Label      string `json:"label"`
	URL        string `json:"url"`
	MonthlyDay int    `json:"monthly_day"` // zero means shortcut only
	DoneMonth  string `json:"done_month,omitempty"`
}
