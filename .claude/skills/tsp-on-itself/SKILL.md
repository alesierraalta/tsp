---
name: tsp-on-itself
description: "Trigger: change to tsp, desarrollar tsp, after a tsp change is green, dogfood, usar tsp sobre sí mismo, haz tsp sobre ti mismo, ¿vamos por buen camino?, ¿tsp nos sirve?. Run TSP on every change to this repository and record what it found, what it cost and what got in the way, so the tool proves it can develop itself."
license: Apache-2.0
metadata:
  author: "alesierraalta"
  version: "1.0"
---

## Activation Contract

Load in this repository when a change to `cmd/`, `internal/`, `assets/skills/`, `bench/` or a project
skill is green and about to be committed or reported done. It runs after the change's own tests pass and
before its PR; `measure-change` still owes its reading. Not for documentation-only edits.

## Hard Rules

1. Green is not done: run `test-strategy` on the diff (the promises it touches and every caller that
   inherits them) before the commit that closes the change. A skipped run is reported as skipped, with
   the reason.
2. Use this worktree's build (`make build`, then `./bin/tsp`) for every `tsp` command, never the
   installed one on PATH: the run must exercise the code under change.
3. Each defect the run finds is pinned RED first, then fixed, then recorded with `tsp plan add-evidence`
   and `tsp plan add-finding` in `docs/testing/test-plan.md`; `tsp plan check` must pass.
4. Submit `tsp feedback --file` for every run with an honest verdict: `paid` only when it found or proved
   something the change would have shipped without; `partly`, `ceremony` and `blocked` are recorded as
   they happened, never smoothed over.
5. Friction is a finding about the tool: a step that needed manual work, a false alarm, a block, text the
   tool changed or lost, a missing flag. Write it in `freeform`; when it would bite the next run, open an
   issue or a plan row in the same session.

## Decision Gates

| Situation | Do |
| --- | --- |
| First change of a session | Read `./bin/tsp feedback -summary` for this repository; a rising share of `partly`, `ceremony` or `blocked` is said before starting |
| The run found a defect in your own fix | Pin, fix, record; the verdict is `paid` |
| The run found nothing | `partly` or `ceremony`; name the probe classes it drove |
| The tool blocked the flow | Stop, record `blocked`, report what blocked it and the decision needed |

## Output Contract

The report of every change names: what the self-run found (`path:line` and the pinning test), what it
cost, the feedback verdict submitted, and the friction recorded.
