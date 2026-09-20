package timewrapper

import (
	"testing"
	"time"
)

func TestClockNowIsCurrent(t *testing.T) {
	t.Parallel()
	before := time.Now()
	got := NewClock().Now()
	after := time.Now()
	if got.Before(before) || got.After(after) {
		t.Fatalf("Now()=%v not in [%v, %v]", got, before, after)
	}
}

func TestClockAfter(t *testing.T) {
	t.Parallel()
	select {
	case <-NewClock().After(time.Millisecond):
	case <-time.After(time.Second):
		t.Fatal("After did not fire")
	}
}

func TestFixed(t *testing.T) {
	t.Parallel()
	base := time.Date(2026, 1, 2, 3, 4, 5, 0, JST())
	f := NewFixed(base)

	if got := f.Now(); !got.Equal(base) {
		t.Fatalf("Now()=%v want %v", got, base)
	}

	// After は待たずに進めた時刻を返す
	select {
	case got := <-f.After(time.Hour):
		if !got.Equal(base.Add(time.Hour)) {
			t.Fatalf("After()=%v want %v", got, base.Add(time.Hour))
		}
	default:
		t.Fatal("After must send immediately")
	}
	if got := f.Now(); !got.Equal(base) {
		t.Fatalf("After must not move Now: %v", got)
	}

	f.Advance(time.Minute)
	if got := f.Now(); !got.Equal(base.Add(time.Minute)) {
		t.Fatalf("Advance: Now()=%v", got)
	}
	f.Set(base)
	if got := f.Now(); !got.Equal(base) {
		t.Fatalf("Set: Now()=%v", got)
	}
}

func TestJST(t *testing.T) {
	t.Parallel()
	if name, offset := time.Date(2026, 1, 1, 0, 0, 0, 0, JST()).Zone(); name != "JST" || offset != 9*60*60 {
		t.Fatalf("zone=%s offset=%d", name, offset)
	}
	if JST() != JST() {
		t.Fatal("JST() must return the shared Location")
	}
}

func TestInJST(t *testing.T) {
	t.Parallel()
	utc := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	got := InJST(utc)
	if got.Location() != JST() || got.Hour() != 9 || !got.Equal(utc) {
		t.Fatalf("InJST(%v)=%v", utc, got)
	}
	if got := InJST(time.Time{}); !got.IsZero() || got.Location() == JST() {
		t.Fatalf("zero value must be returned as is: %v", got)
	}
}
