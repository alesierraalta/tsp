package fetch

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// RetryAfter reads the Retry-After header of a response. The header is either a number of seconds
// or an HTTP date; anything else is refused, because a malformed wait that read as zero would turn
// a rate limit into a tight loop.
func RetryAfter(header string, now time.Time) (time.Duration, error) {
	header = strings.TrimSpace(header)
	if header == "" {
		return 0, fmt.Errorf("fetch: no Retry-After header")
	}
	if seconds, err := strconv.Atoi(header); err == nil {
		if seconds < 0 {
			return 0, fmt.Errorf("fetch: Retry-After %q is negative", header)
		}
		return time.Duration(seconds) * time.Second, nil
	}
	when, err := httpDate(header)
	if err != nil {
		return 0, fmt.Errorf("fetch: Retry-After %q: %w", header, err)
	}
	wait := when.Sub(now)
	if wait < 0 {
		return 0, nil
	}
	return wait, nil
}

// httpDate parses the three date forms an HTTP response may carry.
func httpDate(raw string) (time.Time, error) {
	for _, layout := range []string{time.RFC1123, time.RFC1123Z, time.RFC850, time.ANSIC} {
		if parsed, err := time.Parse(layout, raw); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("not an HTTP date")
}

// Wait bounds a Retry-After value to a ceiling. A service that asks for an hour is answered with the
// ceiling and the queue keeps its own schedule: the endpoint's preference is a hint, not a command.
func Wait(header string, now time.Time, ceiling time.Duration) time.Duration {
	wait, err := RetryAfter(header, now)
	if err != nil || wait <= 0 {
		return 0
	}
	if ceiling > 0 && wait > ceiling {
		return ceiling
	}
	return wait
}
