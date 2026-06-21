package updater

import "testing"

func TestIsNewer(t *testing.T) {
	tests := []struct {
		latest, current string
		want            bool
	}{
		{"v2.0.0", "v1.0.0", true},
		{"v1.0.0", "v2.0.0", false},
		{"v1.0.0", "v1.0.0", false},
		{"v2.1.0", "v2.0.9", true},
		{"v2.0.0", "dev", true},
		{"", "v1.0.0", false},
	}
	for _, tc := range tests {
		if got := isNewer(tc.latest, tc.current); got != tc.want {
			t.Errorf("isNewer(%q, %q) = %v, want %v", tc.latest, tc.current, got, tc.want)
		}
	}
}

func TestSemverCmp(t *testing.T) {
	if semverCmp("v2.0.0", "v1.9.9") <= 0 {
		t.Fatal("expected v2.0.0 > v1.9.9")
	}
	if semverCmp("v1.10.0", "v1.9.0") <= 0 {
		t.Fatal("expected v1.10.0 > v1.9.0")
	}
}

func TestParseSemver(t *testing.T) {
	got := parseSemver("v2.1.3-rc1")
	if got[0] != 2 || got[1] != 1 || got[2] != 3 {
		t.Fatalf("parseSemver = %v", got)
	}
}
