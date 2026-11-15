// Package jobs: the schedule the service runs its queue on.
package jobs

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Schedule is a fixed interval between two runs of the queue.
type Schedule struct {
	Every time.Duration
}

// ParseSchedule reads an interval written as "<n><unit>", where the unit is s, m or h. A schedule
// below the minimum interval is refused: a queue drained every second costs more than it delivers.
func ParseSchedule(raw string) (Schedule, error) {
	raw = strings.TrimSpace(raw)
	if len(raw) < 2 {
		return Schedule{}, fmt.Errorf("jobs: schedule %q", raw)
	}
	unit := raw[len(raw)-1]
	value, err := strconv.Atoi(raw[:len(raw)-1])
	if err != nil || value <= 0 {
		return Schedule{}, fmt.Errorf("jobs: schedule %q", raw)
	}
	var every time.Duration
	switch unit {
	case 's':
		every = time.Duration(value) * time.Second
	case 'm':
		every = time.Duration(value) * time.Minute
	case 'h':
		every = time.Duration(value) * time.Hour
	default:
		return Schedule{}, fmt.Errorf("jobs: schedule unit %q is not s, m or h", string(unit))
	}
	if every < time.Second {
		return Schedule{}, fmt.Errorf("jobs: schedule %q is below one second", raw)
	}
	return Schedule{Every: every}, nil
}

// Next returns the first run after now.
func (s Schedule) Next(now time.Time) time.Time {
	if s.Every <= 0 {
		return now
	}
	return now.Add(s.Every)
}

// String renders the schedule back in the form ParseSchedule reads.
func (s Schedule) String() string {
	switch {
	case s.Every%time.Hour == 0:
		return fmt.Sprintf("%dh", s.Every/time.Hour)
	case s.Every%time.Minute == 0:
		return fmt.Sprintf("%dm", s.Every/time.Minute)
	default:
		return fmt.Sprintf("%ds", s.Every/time.Second)
	}
}
