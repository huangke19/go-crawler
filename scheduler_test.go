package main

import (
	"strings"
	"testing"
	"time"
)

func TestNextScheduledDailyCheckBeforeTarget(t *testing.T) {
	loc := time.FixedZone("CST", 8*3600)
	now := time.Date(2026, 3, 18, 9, 30, 0, 0, loc)

	next := nextScheduledDailyCheck(now, 10, 0)
	want := time.Date(2026, 3, 18, 10, 0, 0, 0, loc)

	if !next.Equal(want) {
		t.Fatalf("expected %v, got %v", want, next)
	}
}

func TestNextScheduledDailyCheckAfterTarget(t *testing.T) {
	loc := time.FixedZone("CST", 8*3600)
	now := time.Date(2026, 3, 18, 10, 0, 1, 0, loc)

	next := nextScheduledDailyCheck(now, 10, 0)
	want := time.Date(2026, 3, 19, 10, 0, 0, 0, loc)

	if !next.Equal(want) {
		t.Fatalf("expected %v, got %v", want, next)
	}
}

func TestTrimScheduledCommandOutput(t *testing.T) {
	input := strings.Repeat("a", scheduledOutputTrimLimit+10)
	got := trimScheduledCommandOutput(input)

	if !strings.HasSuffix(got, "\n...<truncated>") {
		t.Fatalf("expected truncated suffix, got %q", got)
	}
	if len(got) <= scheduledOutputTrimLimit {
		t.Fatalf("expected output to include suffix, got len=%d", len(got))
	}
}
