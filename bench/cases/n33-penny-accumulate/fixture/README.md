# Penny accumulation

`total(rows)` sums each row's decimal-dollar `amount` and returns the total in dollars.
Amounts are decimal strings with exactly two fractional digits, representing integer cents.
The aggregate equals the exact integer-cent sum for every input row.

`formatRowAmount(amount)` rounds each displayed row independently to cents; this presentation does not change the aggregate.
