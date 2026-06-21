//go:build !linux && !darwin

package hoststats

import (
	"context"
	"runtime"
	"time"
)

type platformCollector struct{}

func (platformCollector) Snapshot(ctx context.Context) (Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}

	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	used := mem.Sys
	total := used
	if total == 0 {
		total = 1
	}
	memPct := float64(used) / float64(total) * 100

	return Snapshot{
		CPU:              NewResource(0),
		Memory:           NewResource(memPct),
		MemoryUsedBytes:  used,
		MemoryTotalBytes: total,
		CPUAvailable:     false,
		CollectedAt:      roundCollectedAt(time.Now()),
	}, nil
}
