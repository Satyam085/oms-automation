package utils

import (
	"math"
	"testing"
	"time"
)

// An outage with no restore timestamp is still running; its duration must be
// measured against the same clock frame the occur timestamp was parsed in.
func TestOngoingOutageDuration(t *testing.T) {
	ist, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		t.Skip("no tzdata")
	}
	time.Local = ist

	start := time.Now().Add(-2 * time.Hour)
	got, err := CalculateDurationFromTimestamps(
		start.Format("2006-01-02"), start.Format("15:04:05"), "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if math.Abs(got-2) > 0.01 {
		t.Errorf("duration = %.2fh, want ~2.00h", got)
	}
}

func TestRestoredOutageDuration(t *testing.T) {
	got, err := CalculateDurationFromTimestamps(
		"2026-01-28", "17:37:25.743", "2026-01-28", "20:07:25.743")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if math.Abs(got-2.5) > 0.01 {
		t.Errorf("duration = %.2fh, want 2.50h", got)
	}
}
