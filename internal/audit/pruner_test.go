package audit

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/HopStat/HopStat/internal/domain"
)

// Guarded: Run calls Cleanup from its own goroutine while the test reads the result.
type fakeAuditRepo struct {
	mu      sync.Mutex
	cutoffs []string
	removed int64
	err     error
}

func (f *fakeAuditRepo) seen() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.cutoffs...)
}

func (f *fakeAuditRepo) Log(context.Context, *domain.AuditEntry) error { return nil }
func (f *fakeAuditRepo) List(context.Context, domain.AuditFilter) ([]*domain.AuditEntry, int, error) {
	return nil, 0, nil
}
func (f *fakeAuditRepo) CountToday(context.Context) (int, error) { return 0, nil }
func (f *fakeAuditRepo) Cleanup(_ context.Context, olderThan string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cutoffs = append(f.cutoffs, olderThan)
	return f.removed, f.err
}

func prunerAt(repo domain.AuditRepository, days int, now time.Time) *Pruner {
	p := NewPruner(repo)
	p.SetRetention(func() int { return days })
	p.now = func() time.Time { return now }
	return p
}

func TestPruneOnce_DeletesOlderThanTheWindow(t *testing.T) {
	repo := &fakeAuditRepo{removed: 7}
	now := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)

	removed, err := prunerAt(repo, 90, now).PruneOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if removed != 7 {
		t.Fatalf("removed = %d, want 7", removed)
	}
	// Must match what CURRENT_TIMESTAMP writes, or the text comparison matches nothing.
	if len(repo.seen()) != 1 || repo.seen()[0] != "2025-12-10 12:00:00" {
		t.Fatalf("cutoff = %v", repo.seen())
	}
}

func TestPruneOnce_KeepForeverTouchesNothing(t *testing.T) {
	repo := &fakeAuditRepo{}
	removed, err := prunerAt(repo, KeepForever, time.Now()).PruneOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if removed != 0 || len(repo.seen()) != 0 {
		t.Fatalf("retention 0 must not delete: removed=%d cutoffs=%v", removed, repo.seen())
	}
}

func TestPruneOnce_NoRetentionSourceKeepsEverything(t *testing.T) {
	repo := &fakeAuditRepo{}
	if _, err := NewPruner(repo).PruneOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(repo.seen()) != 0 {
		t.Fatal("an unset retention must not delete anything")
	}
}

func TestPruneOnce_ClampsOutOfRangeRetention(t *testing.T) {
	now := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)

	negative := &fakeAuditRepo{}
	if _, err := prunerAt(negative, -5, now).PruneOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(negative.seen()) != 0 {
		t.Fatal("a negative retention must be treated as keep-forever, not as a deletion")
	}

	huge := &fakeAuditRepo{}
	if _, err := prunerAt(huge, MaxRetentionDays+1000, now).PruneOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := now.AddDate(0, 0, -MaxRetentionDays).Format(sqliteTimestampLayout)
	if len(huge.seen()) != 1 || huge.seen()[0] != want {
		t.Fatalf("cutoff = %v, want it clamped to %s", huge.seen(), want)
	}
}

func TestPruneOnce_ReportsRepositoryFailure(t *testing.T) {
	repo := &fakeAuditRepo{err: errors.New("db gone")}
	if _, err := prunerAt(repo, 30, time.Now()).PruneOnce(context.Background()); err == nil {
		t.Fatal("expected the repository error to surface")
	}
}

func TestRun_PrunesAtStartAndStopsWithTheContext(t *testing.T) {
	repo := &fakeAuditRepo{removed: 1}
	p := prunerAt(repo, 30, time.Now().UTC())

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); p.Run(ctx, time.Hour) }()

	// The first pass happens before the ticker, so it is observable immediately.
	deadline := time.After(2 * time.Second)
	for len(repo.seen()) == 0 {
		select {
		case <-deadline:
			t.Fatal("Run did not prune at startup")
		default:
			time.Sleep(time.Millisecond)
		}
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop when the context was cancelled")
	}
}

func TestRun_TicksAgain(t *testing.T) {
	repo := &fakeAuditRepo{}
	p := prunerAt(repo, 30, time.Now().UTC())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go p.Run(ctx, 2*time.Millisecond)

	deadline := time.After(3 * time.Second)
	for {
		if len(repo.seen()) >= 2 {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("only %d passes; the ticker did not fire", len(repo.seen()))
		default:
			time.Sleep(time.Millisecond)
		}
	}
}

func TestRun_LogsAFailureAndKeepsGoing(t *testing.T) {
	repo := &fakeAuditRepo{err: errors.New("db gone")}
	p := prunerAt(repo, 30, time.Now().UTC())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go p.Run(ctx, 2*time.Millisecond)

	deadline := time.After(3 * time.Second)
	for {
		if len(repo.seen()) >= 2 {
			return
		}
		select {
		case <-deadline:
			t.Fatal("a failing prune stopped the loop")
		default:
			time.Sleep(time.Millisecond)
		}
	}
}

func TestRetentionFromSettings(t *testing.T) {
	cases := []struct {
		name     string
		settings map[string]string
		want     int
	}{
		{"absent", map[string]string{}, 90},
		{"empty", map[string]string{SettingRetentionDays: ""}, 90},
		{"not a number", map[string]string{SettingRetentionDays: "soon"}, 90},
		{"stored", map[string]string{SettingRetentionDays: "30"}, 30},
		{"keep forever", map[string]string{SettingRetentionDays: "0"}, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := RetentionFromSettings(tc.settings, 90); got != tc.want {
				t.Fatalf("got %d, want %d", got, tc.want)
			}
		})
	}
}

func TestNewPrunerUsesTheRealClock(t *testing.T) {
	repo := &fakeAuditRepo{}
	p := NewPruner(repo) // now() not overridden
	p.SetRetention(func() int { return 1 })

	if _, err := p.PruneOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	seen := repo.seen()
	if len(seen) != 1 {
		t.Fatalf("cutoffs = %v", seen)
	}
	cutoff, err := time.Parse(sqliteTimestampLayout, seen[0])
	if err != nil {
		t.Fatalf("cutoff %q is not in the stored timestamp format: %v", seen[0], err)
	}
	if age := time.Since(cutoff); age < 23*time.Hour || age > 25*time.Hour {
		t.Fatalf("cutoff is %s old, want about 24h", age)
	}
}

func TestPruneIntervalIsExposed(t *testing.T) {
	if PruneInterval() != pruneInterval {
		t.Fatalf("PruneInterval() = %s", PruneInterval())
	}
}
