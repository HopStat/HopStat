package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/HopStat/HopStat/internal/hoststats"
)

type stubHostCollector struct {
	snap hoststats.Snapshot
	err  error
}

func (s stubHostCollector) Snapshot(context.Context) (hoststats.Snapshot, error) {
	return s.snap, s.err
}

func TestSystemStatus_Error(t *testing.T) {
	prev := systemStatsCollector
	systemStatsCollector = stubHostCollector{err: errors.New("collect failed")}
	t.Cleanup(func() { systemStatsCollector = prev })

	c, w := setupAdminContext(nil, http.MethodGet, "/admin/system/status", "", 1)
	SystemStatus()(c)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestSystemStatus_Success(t *testing.T) {
	prev := systemStatsCollector
	systemStatsCollector = stubHostCollector{snap: hoststats.Snapshot{
		CPU:              hoststats.NewResource(82.4),
		Memory:           hoststats.NewResource(91.2),
		MemoryUsedBytes:  14_000_000_000,
		MemoryTotalBytes: 16_000_000_000,
		CollectedAt:      time.Now().UTC().Format(time.RFC3339),
	}}
	t.Cleanup(func() { systemStatsCollector = prev })

	c, w := setupAdminContext(nil, http.MethodGet, "/admin/system/status", "", 1)
	SystemStatus()(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var resp struct {
		Data struct {
			CPU struct {
				Percent float64 `json:"percent"`
				Level   string  `json:"level"`
			} `json:"cpu"`
			Memory struct {
				Percent float64 `json:"percent"`
				Level   string  `json:"level"`
			} `json:"memory"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if resp.Data.CPU.Level != "warning" {
		t.Fatalf("cpu level = %q, want warning", resp.Data.CPU.Level)
	}
	if resp.Data.Memory.Level != "critical" {
		t.Fatalf("memory level = %q, want critical", resp.Data.Memory.Level)
	}
}
