# settlement ledger

Money for the settlement ledger, held as integer cents.

- `ParseCents(decimal)` reads an amount written as a decimal string and returns its exact value in cents:
  `"1.15"` is 115. At most two fraction digits are accepted; a third is refused with `ErrFormat`, because
  an amount the ledger cannot hold exactly must not be rounded silently.
- `FormatCents(cents)` writes an amount back as `-1.15`.
- `Split(total, parts)` divides a total into `parts` shares whose sum is exactly `total`; shares differ by at
  most one cent, and `parts` must be positive.
- `ApplyRate(cents, bps)` applies a basis-point rate and rounds half away from zero.
- `Sum(amounts)` adds amounts; `Entry` and `Post` keep a running balance of a journal, and `Balance` reports
  the net amount of a journal.

Run `go test ./...`.
