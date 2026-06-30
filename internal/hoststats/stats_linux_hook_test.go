//go:build linux

package hoststats

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

// ioErrReader returns data on the first Read, then an error on subsequent reads.
type ioErrReader struct {
	data string
	pos  int
}

func (r *ioErrReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, errors.New("injected I/O error")
	}
	n := copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}

func (r *ioErrReader) Close() error { return nil }

func TestReadCPUSampleErrors(t *testing.T) {
	origOpen := linuxOpenStat
	t.Cleanup(func() { linuxOpenStat = origOpen })

	t.Run("open error", func(t *testing.T) {
		linuxOpenStat = func() (io.ReadCloser, error) { return nil, errors.New("open failed") }
		if _, err := readCPUSample(); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("empty file", func(t *testing.T) {
		linuxOpenStat = func() (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader("")), nil
		}
		if _, err := readCPUSample(); err == nil {
			t.Fatal("expected error for empty stat")
		}
	})

	t.Run("bad cpu line", func(t *testing.T) {
		linuxOpenStat = func() (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader("notcpu 1 2 3\n")), nil
		}
		if _, err := readCPUSample(); err == nil {
			t.Fatal("expected error for bad cpu line format")
		}
	})

	t.Run("parse uint error", func(t *testing.T) {
		linuxOpenStat = func() (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader("cpu a b c d e\n")), nil
		}
		if _, err := readCPUSample(); err == nil {
			t.Fatal("expected parse error")
		}
	})
}

func TestCPUPercentLinuxErrors(t *testing.T) {
	origOpen := linuxOpenStat
	t.Cleanup(func() { linuxOpenStat = origOpen })

	t.Run("first sample error", func(t *testing.T) {
		linuxOpenStat = func() (io.ReadCloser, error) { return nil, errors.New("open failed") }
		if _, err := cpuPercentLinux(context.Background(), time.Nanosecond); err == nil {
			t.Fatal("expected error on first sample")
		}
	})

	t.Run("second sample error", func(t *testing.T) {
		var count int
		linuxOpenStat = func() (io.ReadCloser, error) {
			count++
			if count == 1 {
				return io.NopCloser(strings.NewReader("cpu 100 0 0 200 0 0 0 0 0 0\n")), nil
			}
			return nil, errors.New("second open failed")
		}
		if _, err := cpuPercentLinux(context.Background(), time.Nanosecond); err == nil {
			t.Fatal("expected error on second sample")
		}
	})

	t.Run("total delta zero", func(t *testing.T) {
		linuxOpenStat = func() (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader("cpu 100 0 0 200 0 0 0 0 0 0\n")), nil
		}
		pct, err := cpuPercentLinux(context.Background(), time.Nanosecond)
		if err != nil || pct != 0 {
			t.Fatalf("expected 0 pct with zero totalDelta, got pct=%v err=%v", pct, err)
		}
	})

	t.Run("busy negative clamped", func(t *testing.T) {
		var count int
		linuxOpenStat = func() (io.ReadCloser, error) {
			count++
			if count == 1 {
				// idle=50, total=100
				return io.NopCloser(strings.NewReader("cpu 50 0 0 50 0 0 0 0 0 0\n")), nil
			}
			// idle=90, total=120 → idleDelta=40 > totalDelta=20 → busy<0 → clamped to 0
			return io.NopCloser(strings.NewReader("cpu 30 0 0 90 0 0 0 0 0 0\n")), nil
		}
		pct, err := cpuPercentLinux(context.Background(), time.Nanosecond)
		if err != nil || pct != 0 {
			t.Fatalf("expected 0 pct when busy<0, got pct=%v err=%v", pct, err)
		}
	})
}

func TestLoadAvg1LinuxErrors(t *testing.T) {
	origRead := linuxReadLoadavg
	t.Cleanup(func() { linuxReadLoadavg = origRead })

	t.Run("read error", func(t *testing.T) {
		linuxReadLoadavg = func() ([]byte, error) { return nil, errors.New("read failed") }
		if _, err := loadAvg1Linux(); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("empty content", func(t *testing.T) {
		linuxReadLoadavg = func() ([]byte, error) { return []byte{}, nil }
		if _, err := loadAvg1Linux(); err == nil {
			t.Fatal("expected error for empty loadavg")
		}
	})

	t.Run("parse float error", func(t *testing.T) {
		linuxReadLoadavg = func() ([]byte, error) { return []byte("notafloat 0.5 0.3"), nil }
		if _, err := loadAvg1Linux(); err == nil {
			t.Fatal("expected parse error")
		}
	})
}

func TestMemoryLinuxErrors(t *testing.T) {
	origOpen := linuxOpenMeminfo
	t.Cleanup(func() { linuxOpenMeminfo = origOpen })

	t.Run("open error", func(t *testing.T) {
		linuxOpenMeminfo = func() (io.ReadCloser, error) { return nil, errors.New("open failed") }
		if _, _, err := memoryLinux(); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("parse error", func(t *testing.T) {
		linuxOpenMeminfo = func() (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader("MemTotal: bad kB\nMemAvailable: 100 kB\n")), nil
		}
		if _, _, err := memoryLinux(); err == nil {
			t.Fatal("expected parse error for bad MemTotal")
		}
	})

	t.Run("scanner error", func(t *testing.T) {
		linuxOpenMeminfo = func() (io.ReadCloser, error) {
			return &ioErrReader{data: "MemTotal: 1000 kB\n"}, nil
		}
		if _, _, err := memoryLinux(); err == nil {
			t.Fatal("expected scanner I/O error")
		}
	})

	t.Run("mem total not found", func(t *testing.T) {
		linuxOpenMeminfo = func() (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader("SomeOther: 100 kB\n")), nil
		}
		if _, _, err := memoryLinux(); err == nil {
			t.Fatal("expected error when MemTotal not found")
		}
	})

	t.Run("mem available clamped", func(t *testing.T) {
		linuxOpenMeminfo = func() (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader("MemTotal: 1000 kB\nMemAvailable: 2000 kB\n")), nil
		}
		used, total, err := memoryLinux()
		if err != nil || total != 1000*1024 || used != 0 {
			t.Fatalf("clamped: used=%d total=%d err=%v", used, total, err)
		}
	})
}

func TestParseMeminfoKBErrors(t *testing.T) {
	t.Run("too few fields", func(t *testing.T) {
		if _, err := parseMeminfoKB("MemTotal:"); err == nil {
			t.Fatal("expected error for too few fields")
		}
	})

	t.Run("parse uint error", func(t *testing.T) {
		if _, err := parseMeminfoKB("MemTotal: notanumber kB"); err == nil {
			t.Fatal("expected parse error")
		}
	})
}

func TestSnapshotLinuxErrors(t *testing.T) {
	t.Run("cpu error propagates", func(t *testing.T) {
		origOpen := linuxOpenStat
		t.Cleanup(func() { linuxOpenStat = origOpen })
		linuxOpenStat = func() (io.ReadCloser, error) { return nil, errors.New("no stat") }
		if _, err := (platformCollector{}).Snapshot(context.Background()); err == nil {
			t.Fatal("expected error from cpu")
		}
	})

	t.Run("load error propagates", func(t *testing.T) {
		origInterval := snapshotCPUInterval
		origRead := linuxReadLoadavg
		t.Cleanup(func() {
			snapshotCPUInterval = origInterval
			linuxReadLoadavg = origRead
		})
		snapshotCPUInterval = time.Nanosecond
		linuxReadLoadavg = func() ([]byte, error) { return nil, errors.New("no loadavg") }
		if _, err := (platformCollector{}).Snapshot(context.Background()); err == nil {
			t.Fatal("expected error from loadavg")
		}
	})

	t.Run("memory error propagates", func(t *testing.T) {
		origInterval := snapshotCPUInterval
		origOpen := linuxOpenMeminfo
		t.Cleanup(func() {
			snapshotCPUInterval = origInterval
			linuxOpenMeminfo = origOpen
		})
		snapshotCPUInterval = time.Nanosecond
		linuxOpenMeminfo = func() (io.ReadCloser, error) { return nil, errors.New("no meminfo") }
		if _, err := (platformCollector{}).Snapshot(context.Background()); err == nil {
			t.Fatal("expected error from memory")
		}
	})
}
