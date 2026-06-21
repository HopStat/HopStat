package hoststats

import "testing"

func TestLevelForPercent(t *testing.T) {
	tests := []struct {
		percent float64
		want    Level
	}{
		{0, LevelOK},
		{69.9, LevelOK},
		{70, LevelWarning},
		{89.9, LevelWarning},
		{90, LevelCritical},
		{100, LevelCritical},
	}
	for _, tc := range tests {
		if got := LevelForPercent(tc.percent); got != tc.want {
			t.Fatalf("LevelForPercent(%v) = %q, want %q", tc.percent, got, tc.want)
		}
	}
}

func TestNewResourceClamps(t *testing.T) {
	r := NewResource(150)
	if r.Percent != 100 || r.Level != LevelCritical {
		t.Fatalf("NewResource(150) = %+v", r)
	}
	r = NewResource(-5)
	if r.Percent != 0 || r.Level != LevelOK {
		t.Fatalf("NewResource(-5) = %+v", r)
	}
}

func TestCollectLinux(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode")
	}
	snap, err := Collect(t.Context())
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if snap.MemoryTotalBytes == 0 {
		t.Fatal("expected memory total > 0")
	}
	if snap.Memory.Percent < 0 || snap.Memory.Percent > 100 {
		t.Fatalf("memory percent out of range: %v", snap.Memory.Percent)
	}
	if snap.CollectedAt == "" {
		t.Fatal("expected collected_at")
	}
}
