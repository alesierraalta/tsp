# Event replay

`Replay` applies events to a state value.

- Events apply in sequence order; a gap buffers until the missing event arrives.
- Events older than 5 minutes are dropped (documented retention).
- Each event has a `seq` sequence number, a `value`, and an `at` timestamp in milliseconds.
