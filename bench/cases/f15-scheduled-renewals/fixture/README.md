# Scheduled renewals

Small UTC scheduling primitives for a billing worker. Run `node --test`.

`RenewalCalendar(start)` accepts a UTC date-only ISO string and `advance()` returns
its next monthly renewal as a date-only string. Renewals preserve the original
billing day, using the last day of the target month when that day is unavailable.

`RetrySchedule(baseDelay, maxDelay)` accepts positive millisecond delays and
`next()` returns the elapsed offset of the next retry from the initial failure.
Intervals start at the base delay and follow capped exponential backoff, doubling
until the maximum delay is reached. Each instance represents one failure episode.
