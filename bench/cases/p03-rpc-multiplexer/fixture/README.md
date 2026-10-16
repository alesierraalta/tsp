# RPC multiplexer

A library for request-scoped delivery over an ordered response transport.
Each submitted request receives its own response, unless its callback has been
cancelled. Cancellation stops callback delivery without cancelling wire work.

The shared frame format supports reads, replication, and read-only inspection.
Handler generation one supports reads and replication; generation two adds
inspection. Frames may be dispatched to either generation. Inspection must not
change stored data or replication counts.

`Submit` returns a request handle; `Cancel` disables its callback. The injected
transport emits exactly one response per submitted frame in submission order.
Callers advance the transport explicitly with `Receive`, one response at a time.
Callback and transport methods run as individual steps on the caller's scheduler.
`Handle` operates on injected in-memory data.

Run `go test ./...`.
