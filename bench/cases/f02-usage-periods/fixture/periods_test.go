package periods

import (
	"testing"
	"time"
)

func TestWeeklyReports(t *testing.T) {
	at := time.Date(2024, 6, 12, 12, 0, 0, 0, time.UTC)
	if got := WeeklyKey(at); got != "2024-W24" {
		t.Fatalf("key=%s", got)
	}
	if WeeklyKey(at.Add(24*time.Hour)) != WeeklyKey(at) {
		t.Fatal("report keys differ")
	}
}

func TestUsageCharges(t *testing.T) {
	start := time.Date(2024, 6, 12, 12, 0, 0, 0, time.UTC)
	if got := BillableHours(start, start.Add(3*time.Hour)); got != 3 {
		t.Fatalf("hours=%d", got)
	}
	if BillableHours(start, start) != 0 || BillableHours(start, start.Add(-time.Hour)) != 0 {
		t.Fatal("nonpositive interval charged")
	}
}
