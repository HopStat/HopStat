package domain

import (
	"context"
	"time"
)

type ctxKey struct{}

var onLineCtxKey = ctxKey{}

// WithOnLine attaches a streaming line callback to the context.
func WithOnLine(ctx context.Context, onLine func(string)) context.Context {
	return context.WithValue(ctx, onLineCtxKey, onLine)
}

// GetOnLine retrieves the streaming line callback from the context.
func GetOnLine(ctx context.Context) func(string) {
	if fn, ok := ctx.Value(onLineCtxKey).(func(string)); ok {
		return fn
	}
	return nil
}

type QueryStatus string

const (
	StatusPending QueryStatus = "pending"
	StatusRunning QueryStatus = "running"
	StatusDone    QueryStatus = "done"
	StatusError   QueryStatus = "error"
)

type Query struct {
	ID        string       `json:"id"`
	NodeID    int64        `json:"node_id"`
	Command   CommandType  `json:"command"`
	Target    string       `json:"target"`
	Options   QueryOptions `json:"options"`
	SourceIP  string       `json:"source_ip"`
	UserID    *int64       `json:"user_id,omitempty"`
	CreatedAt time.Time    `json:"created_at"`
}

type QueryOptions struct {
	PingCount int    `json:"ping_count"`
	MaxHops   int    `json:"max_hops"`
	MTRCycles int    `json:"mtr_cycles"`
	Protocol  string `json:"protocol,omitempty"`
}

type QueryResult struct {
	ID             string            `json:"id"`
	Status         QueryStatus       `json:"status"`
	Raw            string            `json:"raw,omitempty"`
	Parsed         interface{}       `json:"parsed,omitempty"`
	DurationMS     int64             `json:"duration_ms"`
	LastProgress   interface{}       `json:"last_progress,omitempty"`
	ErrorMsg       string            `json:"error_msg,omitempty"`
	ErrorCode      string            `json:"error_code,omitempty"`
	MatchedRules   []*CommunityRule  `json:"matched_rules,omitempty"`
	ASPathEnriched []ASInfo          `json:"as_path_enriched,omitempty"`
}
