# Batch publish

`Publish([]Step)` applies an ordered batch of reversible I/O operations. Each
`Step` supplies `Apply() error` and `Undo() error`. Successful applications are
tracked by the publisher. An unsuccessful application must leave no change of its
own; the caller is responsible for that property.

If an application fails, all previously successful steps are undone in reverse
order before returning. Every applicable undo is attempted even if another undo
fails. With successful undos, externally visible state equals its state before
the batch. Undo failures mean state restoration is not guaranteed.

The returned error preserves the application failure and every undo failure for
`errors.Is` inspection. Success returns nil. There are no automatic retries and
no concurrent callbacks. An empty batch succeeds without callbacks.

Run `go test ./...`. The module uses only the standard library.
