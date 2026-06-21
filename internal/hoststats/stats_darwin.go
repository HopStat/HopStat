//go:build darwin

package hoststats

import (
	"context"
	"encoding/binary"
	"fmt"
	"time"

	"golang.org/x/sys/unix"
)

var (
	darwinSysctlRaw    = unix.SysctlRaw
	darwinSysctlUint64 = unix.SysctlUint64
	darwinSysctlUint32 = unix.SysctlUint32
)

const darwinLoadavgScale = 2048.0

type platformCollector struct{}

func (platformCollector) Snapshot(ctx context.Context) (Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}

	cpu, load1, err := cpuMetricsDarwin()
	if err != nil {
		return Snapshot{}, err
	}
	memUsed, memTotal, err := memoryDarwin()
	if err != nil {
		return Snapshot{}, err
	}
	memPct := 0.0
	if memTotal > 0 {
		memPct = float64(memUsed) / float64(memTotal) * 100
	}
	cores := cpuCores()
	return Snapshot{
		CPU:              NewResource(cpu),
		Memory:           NewResource(memPct),
		MemoryUsedBytes:  memUsed,
		MemoryTotalBytes: memTotal,
		CPUCores:         cores,
		CPULoad1:         roundLoad(load1),
		CPUAvailable:     true,
		CollectedAt:      roundCollectedAt(time.Now()),
	}, nil
}

func cpuMetricsDarwin() (percent, load1 float64, err error) {
	load1, err = loadAvg1Darwin()
	if err != nil {
		return 0, 0, err
	}
	cores := float64(cpuCores())
	return load1 / cores * 100, load1, nil
}

func loadAvg1Darwin() (float64, error) {
	raw, err := darwinSysctlRaw("vm.loadavg")
	if err != nil {
		return 0, err
	}
	if len(raw) < 4 {
		return 0, fmt.Errorf("vm.loadavg: short read (%d bytes)", len(raw))
	}
	return float64(binary.LittleEndian.Uint32(raw[:4])) / darwinLoadavgScale, nil
}

func memoryDarwin() (used, total uint64, err error) {
	total, err = darwinSysctlUint64("hw.memsize")
	if err != nil {
		return 0, 0, err
	}
	pageSize, err := darwinSysctlUint64("hw.pagesize")
	if err != nil {
		return 0, 0, err
	}

	free, err := darwinSysctlUint32("vm.page_free_count")
	if err != nil {
		return 0, 0, err
	}
	purgeable, _ := darwinSysctlUint32("vm.page_purgeable_count")
	speculative, _ := darwinSysctlUint32("vm.page_speculative_count")

	freeBytes := (uint64(free) + uint64(purgeable) + uint64(speculative)) * pageSize
	if freeBytes > total {
		freeBytes = total
	}
	return total - freeBytes, total, nil
}
