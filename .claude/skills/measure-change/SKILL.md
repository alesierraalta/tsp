---
name: measure-change
description: "Trigger: medir, medición, measure, baseline, A/B, ¿esto mejora?, probar y medir, prueba mide prueba mide, después de implementar. Route every change to the reading it owes, compare it paired against a baseline, and record what the reading proved."
license: Apache-2.0
metadata:
  author: "alesierraalta"
  version: "1.0"
---

## Activation Contract

Load when a change to `cmd/`, `internal/`, `assets/skills/`, `bench/` or a project skill is about to be
reported as done, or when the operator asks whether it actually improved. Not for reading stored
numbers, documentation edits, or the corpus loop (`bench-loop`).

## Hard Rules

1. Name the metric and its accept threshold **before** the change; one chosen after the result is not a
   metric.
2. No number without its instrument: commit or version, corpus digest, model, runs per
   side. One run is a reading, not a baseline.
3. Paired only — same instrument, flags and machine, two runs per side, never against a stored old
   number; a delta inside the noise band is not an improvement.
4. `exit 2`, `not measured`, `no engine` and an abstention are never a pass; a rejected change is
   reverted, not argued with.
5. Every reading records what it does **not** prove.

## Decision Gates

| Touched | The reading it owes |
| --- | --- |
| `cmd/`, `internal/` | `make vet`, `make test`, `tsp mutate` on touched lines |
| `internal/gate` | `make mutants` |
| `assets/skills/<skill>/` | `tsp bench run --runs 2`, then `bench compare` |
| plan template, ledger, refusals | `go test ./internal/plan ./internal/assets` |
| `bench/` harness or corpus | re-baseline: a corpus change starts a new series |
| verdict-quality gain | `bench-loop`, train/held-out split |
| a rule with no instrument | build the instrument, or report that no reading exists |

Exact commands, flags and paired recipe: `references/routing.md`.

## Execution Steps

1. **Route** with the table above; `references/routing.md` has the exact command.
2. **Declare** metric, threshold and command in one line: task or commit message.
3. **Baseline** at the base commit: `assets/paired.sh <base-ref> "<cmd>"` runs both sides in a
   throwaway worktree and diffs stdout, stderr, exit status. `-n` normalizes the instrument's own
   noise, never the assertions.
4. **Change once**, so the delta has one cause; **re-measure** identically.
5. **Verdict:** accept only past the threshold, outside the noise band; else revert.
6. **Record** in `docs/measurements/<topic>.md` from `assets/measurement-record.md`, or a
   `bench/history` row.

## Output Contract

Report the declared metric, both sides' commands and results, the verdict and the noise
band it cleared, where the reading was recorded or why none exists, and **what this reading does not
prove**. Close with the `Risk:` line.

## References

- `references/routing.md` — every command, flag and paired recipe, by change class.
- `bench/README.md` — scoring, the baseline, compare refusals, the fix loop.
- `docs/measurements/` — worked readings, each ending in what it refused to claim.
- `assets/paired.sh`, `assets/measurement-record.md`.
