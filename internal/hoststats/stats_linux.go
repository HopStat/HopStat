//go:build linux

package hoststats

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
)

type platformCollector struct{}

var (
	linuxOpenStat       = func() (io.ReadCloser, error) { return os.Open("/proc/stat") }
	linuxOpenMeminfo    = func() (io.ReadCloser, error) { return os.Open("/proc/meminfo") }
	linuxReadLoadavg    = func() ([]byte, error) { return os.ReadFile("/proc/loadavg") }
	snapshotCPUInterval = 200 * time.Millisecond
)

func (platformCollector) Snapshot(ctx context.Context) (Snapshot, error) {
	cpu, err := cpuPercentLinux(ctx, snapshotCPUInterval)
	if err != nil {
		return Snapshot{}, err
	}
	load1, err := loadAvg1Linux()
	if err != nil {
		return Snapshot{}, err
	}
	memUsed, memTotal, err := memoryLinux()
	if err != nil {
		return Snapshot{}, err
	}
	memPct := 0.0
	if memTotal > 0 {
		memPct = float64(memUsed) / float64(memTotal) * 100
	}
	return Snapshot{
		CPU:              NewResource(cpu),
		Memory:           NewResource(memPct),
		MemoryUsedBytes:  memUsed,
		MemoryTotalBytes: memTotal,
		CPUCores:         cpuCores(),
		CPULoad1:         roundLoad(load1),
		CPUAvailable:     true,
		CollectedAt:      roundCollectedAt(time.Now()),
	}, nil
}

type cpuSample struct {
	idle  uint64
	total uint64
}

func readCPUSample() (cpuSample, error) {
	f, err := linuxOpenStat()
	if err != nil {
		return cpuSample{}, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	if !scanner.Scan() {
		return cpuSample{}, fmt.Errorf("read /proc/stat: empty")
	}
	fields := strings.Fields(scanner.Text())
	if len(fields) < 5 || fields[0] != "cpu" {
		return cpuSample{}, fmt.Errorf("unexpected /proc/stat cpu line")
	}

	var total uint64
	for i := 1; i < len(fields); i++ {
		v, err := strconv.ParseUint(fields[i], 10, 64)
		if err != nil {
			return cpuSample{}, fmt.Errorf("parse /proc/stat: %w", err)
		}
		total += v
	}
	// fields[4] (idle) was already validated by the loop above.
	idle, _ := strconv.ParseUint(fields[4], 10, 64)
	return cpuSample{idle: idle, total: total}, scanner.Err()
}

func cpuPercentLinux(ctx context.Context, interval time.Duration) (float64, error) {
	first, err := readCPUSample()
	if err != nil {
		return 0, err
	}
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	case <-timer.C:
	}
	second, err := readCPUSample()
	if err != nil {
		return 0, err
	}

	idleDelta := float64(second.idle - first.idle)
	totalDelta := float64(second.total - first.total)
	if totalDelta <= 0 {
		return 0, nil
	}
	busy := totalDelta - idleDelta
	if busy < 0 {
		busy = 0
	}
	return busy / totalDelta * 100, nil
}

func loadAvg1Linux() (float64, error) {
	data, err := linuxReadLoadavg()
	if err != nil {
		return 0, err
	}
	fields := strings.Fields(string(data))
	if len(fields) < 1 {
		return 0, fmt.Errorf("invalid /proc/loadavg")
	}
	load1, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0, fmt.Errorf("parse /proc/loadavg: %w", err)
	}
	return load1, nil
}

func memoryLinux() (used, total uint64, err error) {
	f, err := linuxOpenMeminfo()
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()

	var memTotal, memAvailable uint64
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "MemTotal:"):
			memTotal, err = parseMeminfoKB(line)
		case strings.HasPrefix(line, "MemAvailable:"):
			memAvailable, err = parseMeminfoKB(line)
		}
		if err != nil {
			return 0, 0, err
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, 0, err
	}
	if memTotal == 0 {
		return 0, 0, fmt.Errorf("MemTotal not found in /proc/meminfo")
	}
	if memAvailable > memTotal {
		memAvailable = memTotal
	}
	used = memTotal - memAvailable
	return used, memTotal, nil
}

func parseMeminfoKB(line string) (uint64, error) {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return 0, fmt.Errorf("invalid meminfo line: %q", line)
	}
	kb, err := strconv.ParseUint(fields[1], 10, 64)
	if err != nil {
		return 0, err
	}
	return kb * 1024, nil
}
