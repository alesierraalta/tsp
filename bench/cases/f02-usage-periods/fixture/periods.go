package periods

import (
	"fmt"
	"time"
)

func WeeklyKey(at time.Time) string {
	_, week := at.UTC().ISOWeek()
	return fmt.Sprintf("%04d-W%02d", at.UTC().Year(), week)
}

func BillableHours(start, end time.Time) int64 {
	if !end.After(start) {
		return 0
	}
	elapsed := end.Sub(start)
	return int64(elapsed / time.Hour)
}
