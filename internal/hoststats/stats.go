package hoststats

import (
	"context"
	"runtime"
	"time"
)

type Level string

const (
	LevelOK       Level = "ok"
	LevelWarning  Level = "warning"
	LevelCritical Level = "critical"
)

const (
	warningThreshold  = 70.0
	criticalThreshold = 90.0
)

type Resource struct {
	Percent float64 `json:"percent"`
	Level   Level   `json:"level"`
}

type Snapshot struct {
	CPU              Resource `json:"cpu"`
	Memory           Resource `json:"memory"`
	MemoryUsedBytes  uint64   `json:"memory_used_bytes"`
	MemoryTotalBytes uint64   `json:"memory_total_bytes"`
	CPUCores         int      `json:"cpu_cores"`
	CPULoad1         float64  `json:"cpu_load_1"`
	CPUAvailable     bool     `json:"cpu_available"`
	CollectedAt      string   `json:"collected_at"`
}

type Collector interface {
	Snapshot(ctx context.Context) (Snapshot, error)
}

func LevelForPercent(percent float64) Level {
	switch {
	case percent >= criticalThreshold:
		return LevelCritical
	case percent >= warningThreshold:
		return LevelWarning
	default:
		return LevelOK
	}
}

func NewResource(percent float64) Resource {
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	return Resource{
		Percent: round1(percent),
		Level:   LevelForPercent(percent),
	}
}

func round1(v float64) float64 {
	return float64(int(v*10+0.5)) / 10
}

func Collect(ctx context.Context) (Snapshot, error) {
	return defaultCollector.Snapshot(ctx)
}

var defaultCollector Collector = platformCollector{}

func roundLoad(v float64) float64 {
	return float64(int(v*100+0.5)) / 100
}

func cpuCores() int {
	cores := numCPUFunc()
	if cores <= 0 {
		return 1
	}
	return cores
}

var numCPUFunc = runtime.NumCPU

func roundCollectedAt(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}
