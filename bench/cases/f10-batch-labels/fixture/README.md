# Batch labels

`VisitIDs` calls the visitor synchronously with each uint64 encoded as eight big-endian bytes. The visitor must not retain or mutate the buffer after returning. Each batch uses at most one buffer allocation, independent of batch length.

`NewLabels(limit, render)` accepts a positive limit and caches labels with FIFO replacement. Retained entries never exceed the limit; hits reuse the label without calling render. `Size` reports retained entries. Instances are used by one goroutine.
