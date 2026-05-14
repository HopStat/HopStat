package domain

import "time"

type Severity string

const (
	SeverityReject   Severity = "reject"
	SeverityWarning  Severity = "warning"
	SeverityInfo     Severity = "info"
	SeveritySuccess  Severity = "success"
)

type CommunityRule struct {
	ID          int64     `json:"id"`
	Community   string    `json:"community"`
	Severity    Severity  `json:"severity"`
	MessageI18n string    `json:"message_i18n"`
	Scope       string    `json:"scope"`
	NodeID      *int64    `json:"node_id,omitempty"`
	Active      bool      `json:"active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
