# Local daily scheduler

The scheduler selects one run for each local calendar date at the configured wall-clock time.
If that time falls in a daylight-saving gap, it uses the first valid local minute after the requested time.
The `now` option accepts an injectable clock function for deterministic schedule calculations.
The optional `withJitter` helper applies an independent random offset of up to ±5 minutes to a scheduled time.
