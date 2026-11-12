# Surface inventory A/B on the u* corpus (#446)

Does starting a run from `tsp probe surface` help a small model find more defects? On this corpus, with
this model: no measurable effect. The model already finds every planted defect without it.

## Instrument

| field | arm A | arm B |
|---|---|---|
| skill | test-strategy 0.7.23 (no surface step) | test-strategy 0.7.24 (step 1 starts from `tsp probe surface`) |
| build | 404c79a7 (no `probe surface`) | 2cb1d3b2 (`probe surface` with Go AST, JS/TS and Python heuristics) |
| model, runner | opencode-go/deepseek-v4.1-flash, pi | same |
| cases, runs | `bench/cases/u0*` (8 cases, 13 defects, 1 clean control), 3 runs each | same |
| agent config, concurrency | bench, 4 | same |
| corpus digest | sha256:c11012f09e8a7194 | same |
| probe executed | n/a | 22 of 24 runs printed `surface: N row(s)` |

Split (`bench/postmortems/split.json`, batch4, recorded before any measurement): u01-u02 train, u03-u08 measure
only. The skill text of arm B was written from one train miss only (u02 D2 merged into D1 in 2 of 3 arm A runs).

## Result

| measure | arm A | arm B |
|---|---|---|
| reported (defect-runs) | 39/39 | 39/39 |
| caught (defect-runs) | 36/39 | 36/39 |
| caught, train u01-u02 | 10/12 | 12/12 |
| caught, held-out u03-u07 | 29/30 | 27/30 |
| false positives, inventions on controls | 0, 0 | 0, 0 |
| turns | 934 | 943 |
| cost (USD) | 0.928 | 0.962 |

`tsp bench compare --skill-versions`: "caught difference 36/39 vs 36/39 is within noise (95% Wilson/Newcombe
interval: -0.136 to +0.136)". The held-out difference (-2 defect-runs: u04 once, u07 twice) is inside the same
noise; the train gain is the case the skill text was written from, so it shows the text does what it says, not
that it generalizes.

## What this does and does not show

- The corpus is at ceiling for this model: arm A detected 39/39. Modules of 80-190 lines are read whole, so an
  inventory has nothing left to point at. A recall gain can only show on code too large to read in one pass.
- The probe costs about 1% more turns and 4% more cost here, inside run-to-run noise (turns CV ~2%, cost CV
  ~9% in earlier measurements).
- The probe's own reach on these cases (T3/T3b workers, train and older cases only): 4 of 13 defect lines sit
  within 3 lines of a row; u01's weak guard is shown only through the guard body
  (`!r.inside(joined) -> strings.HasPrefix(p, r.root)`).
- A first arm B run is excluded: its build was saved as `tsp-surface`, so every agent ran the installed `tsp`
  0.9.0 and reported "probe surface isn't available". The bench now links a renamed build as `tsp` (8ecf9d66).

## Next measurement worth running

A multi-module corpus (thousands of lines, many guarded inputs, the defect in one of them) with the same two
arms, so the inventory has room to change what a small model reads first.
