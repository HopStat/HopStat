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

// NewResource used to round the reported percentage but classify its severity from the
// unrounded input, so a reading a hair under a threshold was displayed as that threshold
// while carrying the lower level — "70%" shown as ok, "90%" shown as warning. Both
// collectors feed it values that routinely land there (load1/cores*100, used/total*100).
func TestNewResourceLevelMatchesReportedPercent(t *testing.T) {
	tests := []struct {
		name      string
		percent   float64
		want      float64
		wantLevel Level
	}{
		{name: "just under the warning threshold", percent: 69.96, want: 70.0, wantLevel: LevelWarning},
		{name: "exactly the warning threshold", percent: 70.0, want: 70.0, wantLevel: LevelWarning},
		{name: "clearly below the warning threshold", percent: 69.9, want: 69.9, wantLevel: LevelOK},
		{name: "just under the critical threshold", percent: 89.96, want: 90.0, wantLevel: LevelCritical},
		{name: "exactly the critical threshold", percent: 90.0, want: 90.0, wantLevel: LevelCritical},
		{name: "just under the lower band edge", percent: 0.04, want: 0.0, wantLevel: LevelOK},
		{name: "zero", percent: 0, want: 0, wantLevel: LevelOK},
		{name: "full", percent: 100, want: 100, wantLevel: LevelCritical},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewResource(tt.percent)
			if got.Percent != tt.want {
				t.Fatalf("NewResource(%v).Percent = %v, want %v", tt.percent, got.Percent, tt.want)
			}
			if got.Level != tt.wantLevel {
				t.Fatalf("NewResource(%v) = {%.1f%% %s}; reporting %.1f%% must be %q",
					tt.percent, got.Percent, got.Level, got.Percent, tt.wantLevel)
			}
		})
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
