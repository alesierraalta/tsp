# Inventory worker

Inventory sells a fixed stock to concurrent buyers.

- `Buy()` returns true when it sells one unit and false when none remain.
- Sold units may never exceed capacity, even with concurrent buyers.
- `Stock()` is clamped at 0 and never shown negative (documented).

The optional `yieldBeforeAct` callback is test-support injection: the fixture invokes it
between its availability check and separate decrement, letting the trigger hold buyers at
that seam. `fix/all` invokes the same callback immediately before its single critical
section, preserving concurrent pressure without splitting the check from the act.
