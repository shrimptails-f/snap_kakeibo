package main

import (
	"testing"
	"time"
)

func TestSampleTimeClampsFutureDaysInCurrentMonth(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 10, 8, 0, 0, 0, time.UTC)

	if got := sampleTime(now, sample{monthOffset: 0, day: 25}); got.Day() != 10 || got.Month() != time.September {
		t.Errorf("current month future day = %v, want clamped to today", got)
	}
	if got := sampleTime(now, sample{monthOffset: -1, day: 25}); got.Day() != 25 || got.Month() != time.August {
		t.Errorf("previous month = %v, want 2026-08-25", got)
	}
	if got := sampleTime(now, sample{monthOffset: -2, day: 31}); got.Day() != 31 || got.Month() != time.July {
		t.Errorf("two months ago = %v, want 2026-07-31", got)
	}
}

func TestSampleIDIsStableForTheSameMonth(t *testing.T) {
	t.Parallel()
	s := sample{monthOffset: -1, day: 3}
	early := sampleID(time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), s, 9)
	late := sampleID(time.Date(2026, 9, 30, 23, 0, 0, 0, time.UTC), s, 9)
	if early != "sample-202608-10" || late != early {
		t.Errorf("ids = %q / %q, want the same sample-202608-10", early, late)
	}
}

func TestSamplesUseValidCategoriesAndAmounts(t *testing.T) {
	t.Parallel()
	for i, s := range samples {
		if s.outcome != outcomeSucceeded {
			continue
		}
		var sum int64
		for _, d := range s.details {
			sum += d.Amount * d.Quantity
		}
		if sum != s.amount {
			t.Errorf("samples[%d] %s: details sum %d != amount %d", i, s.store, sum, s.amount)
		}
	}
}
