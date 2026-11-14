# plan gaps: a layer with two owners (#455)

Declared before the change: on a plan fresh from `tsp plan init`, count the `plan gaps` lines carrying a stray
quote mark. Accept at 0, with every other line and the exit status identical to the base.

## Instrument

| field | base | fix |
|---|---|---|
| build | origin/main a79bdb65 | fix/455-gaps-multi-skill |
| reading | `tsp plan init` in a fresh `git init` directory, then `tsp plan gaps` | same |
| runs | 2, byte-identical | 2, byte-identical |

## Result

| measure | base | fix |
|---|---|---|
| lines with a stray quote mark | 1 | 0 |
| exit status | 1 | 1 |
| other lines | — | identical (`diff` shows only line 42) |

Line 42 of the shipped template, before and after:

    Runtime and faults (runtime-reliability-testing', 'resilience-fault-injection)
    Runtime and faults (runtime-reliability-testing, resilience-fault-injection)

Mutation check: restoring `strings.Trim` on the owner cell turns `internal/plan` red through
`TestGapsNameEveryOwnerOfALayerCleanly`; the fix turns it green. The full suite passes.

## What this does not show

It is a readability fix in one message. It does not change which layers count as owed, the swept ratio, the
exit status, or anything a model decides; the gate counts `assigned and never invoked:` lines, and their number
is unchanged. A layer label (column one) is still sanitised exactly as before, backticks included.
