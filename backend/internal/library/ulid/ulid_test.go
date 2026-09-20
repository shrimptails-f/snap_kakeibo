package ulid

import (
	"testing"
	"time"

	"snap_kakeibo/backend/internal/library/timewrapper"

	oklog "github.com/oklog/ulid/v2"
)

func TestNewIDUsesClockAndStaysUnique(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	ids := New(timewrapper.NewFixed(now))
	seen := map[string]struct{}{}
	for range 100 {
		id, err := ids.NewID()
		if err != nil {
			t.Fatal(err)
		}
		parsed, err := oklog.ParseStrict(id)
		if err != nil {
			t.Fatalf("NewID() = %q is not a ULID: %v", id, err)
		}
		if got := parsed.Time(); got != oklog.Timestamp(now) {
			t.Fatalf("timestamp = %d, want %d", got, oklog.Timestamp(now))
		}
		if _, dup := seen[id]; dup {
			t.Fatalf("NewID() repeated %q", id)
		}
		seen[id] = struct{}{}
	}
}

func TestNewDefaultsToRealClock(t *testing.T) {
	t.Parallel()
	id, err := New(nil).NewID()
	if err != nil || len(id) != 26 {
		t.Fatalf("NewID() = %q, %v", id, err)
	}
}
