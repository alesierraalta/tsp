# Fee splitter

`split(total, partCount)` divides a non-negative whole-unit fee into integer shares.
Parts always sum to the total exactly.
More than 100 parts is rejected with a usage error.
