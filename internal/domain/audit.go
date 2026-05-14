package domain

import "time"

type AuditEntry struct {
	ID         int64      `json:"id"`
	CreatedAt  time.Time  `json:"created_at"`
	SourceIP   string     `json:"source_ip"`
	UserID     *int64     `json:"user_id,omitempty"`
	NodeID     *int64     `json:"node_id,omitempty"`
	Command    string     `json:"command"`
	Params     string     `json:"params"`
	DurationMS int64      `json:"duration_ms"`
	Success    bool       `json:"success"`
	ErrorMsg   string     `json:"error_msg,omitempty"`
}

type AuditFilter struct {
	From     string `json:"from,omitempty"`
	To       string `json:"to,omitempty"`
	NodeID   *int64 `json:"node_id,omitempty"`
	Command  string `json:"command,omitempty"`
	SourceIP string `json:"source_ip,omitempty"`
	Page     int    `json:"page"`
	Limit    int    `json:"limit"`
}
