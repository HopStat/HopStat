//go:build darwin

package hoststats

import (
	"context"
	"encoding/binary"
	"errors"
	"testing"
)

func TestSnapshotDarwinErrors(t *testing.T) {
	t.Run("cpu metrics error", func(t *testing.T) {
		old := darwinSysctlRaw
		darwinSysctlRaw = func(string, ...int) ([]byte, error) { return nil, errors.New("loadavg failed") }
		t.Cleanup(func() { darwinSysctlRaw = old })
		if _, err := (platformCollector{}).Snapshot(context.Background()); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("short loadavg", func(t *testing.T) {
		old := darwinSysctlRaw
		darwinSysctlRaw = func(string, ...int) ([]byte, error) { return []byte{1, 2}, nil }
		t.Cleanup(func() { darwinSysctlRaw = old })
		if _, err := loadAvg1Darwin(); err == nil {
			t.Fatal("expected short read error")
		}
	})

	t.Run("memory sysctl errors", func(t *testing.T) {
		oldRaw := darwinSysctlRaw
		oldU64 := darwinSysctlUint64
		darwinSysctlRaw = func(string, ...int) ([]byte, error) {
			buf := make([]byte, 4)
			binary.LittleEndian.PutUint32(buf, 2048)
			return buf, nil
		}
		darwinSysctlUint64 = func(name string, _ ...int) (uint64, error) {
			if name == "hw.memsize" {
				return 0, errors.New("memsize failed")
			}
			return 4096, nil
		}
		t.Cleanup(func() {
			darwinSysctlRaw = oldRaw
			darwinSysctlUint64 = oldU64
		})
		if _, _, err := memoryDarwin(); err == nil {
			t.Fatal("expected memsize error")
		}
	})

	t.Run("pagesize error", func(t *testing.T) {
		oldRaw := darwinSysctlRaw
		oldU64 := darwinSysctlUint64
		darwinSysctlRaw = func(string, ...int) ([]byte, error) {
			buf := make([]byte, 4)
			binary.LittleEndian.PutUint32(buf, 2048)
			return buf, nil
		}
		darwinSysctlUint64 = func(name string, _ ...int) (uint64, error) {
			if name == "hw.memsize" {
				return 4096, nil
			}
			return 0, errors.New("pagesize failed")
		}
		t.Cleanup(func() {
			darwinSysctlRaw = oldRaw
			darwinSysctlUint64 = oldU64
		})
		if _, _, err := memoryDarwin(); err == nil {
			t.Fatal("expected pagesize error")
		}
	})

	t.Run("page free count error", func(t *testing.T) {
		oldRaw := darwinSysctlRaw
		oldU64 := darwinSysctlUint64
		oldU32 := darwinSysctlUint32
		darwinSysctlRaw = func(string, ...int) ([]byte, error) {
			buf := make([]byte, 4)
			binary.LittleEndian.PutUint32(buf, 2048)
			return buf, nil
		}
		darwinSysctlUint64 = func(name string, _ ...int) (uint64, error) {
			switch name {
			case "hw.memsize":
				return 8192, nil
			case "hw.pagesize":
				return 4096, nil
			default:
				return 0, nil
			}
		}
		darwinSysctlUint32 = func(name string) (uint32, error) {
			if name == "vm.page_free_count" {
				return 0, errors.New("free failed")
			}
			return 0, nil
		}
		t.Cleanup(func() {
			darwinSysctlRaw = oldRaw
			darwinSysctlUint64 = oldU64
			darwinSysctlUint32 = oldU32
		})
		if _, _, err := memoryDarwin(); err == nil {
			t.Fatal("expected free count error")
		}
	})

	t.Run("free bytes clamped", func(t *testing.T) {
		oldRaw := darwinSysctlRaw
		oldU64 := darwinSysctlUint64
		oldU32 := darwinSysctlUint32
		darwinSysctlRaw = func(string, ...int) ([]byte, error) {
			buf := make([]byte, 4)
			binary.LittleEndian.PutUint32(buf, 2048)
			return buf, nil
		}
		darwinSysctlUint64 = func(name string, _ ...int) (uint64, error) {
			switch name {
			case "hw.memsize":
				return 4096, nil
			case "hw.pagesize":
				return 4096, nil
			default:
				return 0, nil
			}
		}
		darwinSysctlUint32 = func(name string) (uint32, error) {
			if name == "vm.page_free_count" {
				return 10, nil
			}
			return 0, nil
		}
		t.Cleanup(func() {
			darwinSysctlRaw = oldRaw
			darwinSysctlUint64 = oldU64
			darwinSysctlUint32 = oldU32
		})
		used, total, err := memoryDarwin()
		if err != nil || total != 4096 || used != 0 {
			t.Fatalf("used=%d total=%d err=%v", used, total, err)
		}
	})

	t.Run("memory error", func(t *testing.T) {
		oldRaw := darwinSysctlRaw
		oldU64 := darwinSysctlUint64
		darwinSysctlRaw = func(string, ...int) ([]byte, error) {
			buf := make([]byte, 4)
			binary.LittleEndian.PutUint32(buf, 2048)
			return buf, nil
		}
		darwinSysctlUint64 = func(name string, _ ...int) (uint64, error) {
			return 0, errors.New("mem failed")
		}
		t.Cleanup(func() {
			darwinSysctlRaw = oldRaw
			darwinSysctlUint64 = oldU64
		})
		if _, err := (platformCollector{}).Snapshot(context.Background()); err == nil {
			t.Fatal("expected memory error")
		}
	})
}
