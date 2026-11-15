# batch decoder

Decodes the row blocks the ingest endpoint receives from partner feeds.

A block is a header line followed by one row per line:

```text
v1 cols=3 rows=2
shirt|7
socks|3
```

- `ParseHeader(line)` reads `v1 cols=<n> rows=<n>`, where `rows` counts the row lines that follow. The
  column count may not exceed `MaxColumns` and the declared row count may not exceed `MaxDeclared`; anything larger is refused with `ErrTooMany` so a partner
  cannot ask the decoder to size a block on its behalf. A negative or unparseable count is refused with
  `ErrFormat`.
- `Decode(body)` returns the rows of one block. A malformed line refuses the whole block with `ErrFormat`,
  so a partially decoded block is never handed to the writer.
- A row line is `label|value` and may carry a repeat factor: `socks|3 x4` is four socks rows. The factor is
  part of the row, and a factor above `MaxRepeat` is refused with `ErrTooMany`.
- `ParseRow(line)` returns one row and its repeat factor. `Encode(rows)` writes a block back out and must
  round-trip what `Decode` accepted.
- `Summary(rows)` counts rows and value per label, sorted by label.

Run `go test ./...`.
