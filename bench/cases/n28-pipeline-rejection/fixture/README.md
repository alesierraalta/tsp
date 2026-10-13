# worker-pipeline

Runs independent work items for a background worker.

- `runPipeline(items, handler, options)` returns successful results in input order.
- If a handler fails, the caller receives an error carrying the failed item's context.
- A per-item timeout logs and skips the item; this is a documented trade-off.
