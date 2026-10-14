# Usage periods

`WeeklyKey(at)` returns the UTC ISO week-date year and week in YYYY-Www format for grouping usage reports. All instants in the same ISO week have the same key.

`BillableHours(start, end)` returns the number of started hours in a usage interval: each positive partial hour is charged as a full hour. Empty or reversed intervals cost zero. It measures elapsed time between instants, independently of timezone. Intervals fit within a Go time.Duration.

Run `go test ./...`. The package uses only the standard library.
