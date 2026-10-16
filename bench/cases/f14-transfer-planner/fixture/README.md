# Transfer planner

`Fetch(keys, read)` returns values in request order. The reader is stable for the
call's duration. Repeated keys share a single reader invocation across the entire
call, including keys whose value is empty; each invocation represents remote I/O.

`Send(data, block, write)` forwards the original bytes in order, in maximal chunks
of at most `block` bytes. Each writer invocation represents one remote I/O operation;
only payload-bearing chunks are sent. A nonpositive block size panics.

Run `go test ./...`. This module uses only the standard library.
