# Surface inventory A/B on the u* and w* corpora (#446, #447)

Does starting a run from `tsp probe surface` help a small model find more defects? On both corpora, with
this model: no measurable effect. The model already finds every planted defect without it, up to at least
4,000 source lines per module.

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

## Second A/B: the multi-module corpus w01-w07 (#447)

Seven cases of 2,507-3,997 non-test source lines across 21-35 files, each routing many inputs through one
correct guard with one or two entry points that skip it, plus a clean control whose guards read weak and
are correct. Same model, runner, runs and concurrency; arm A built from origin/main (skill 0.7.23, no
probe), arm B from the branch (skill 0.7.24, probe). A first w* run labeled "baseline" was mislabeled: its
binary was built from the branch, so it was arm B; the pair below is the clean one.

| measure | arm A (no surface) | arm B (surface) |
|---|---|---|
| caught (defect-runs) | 24/24 | 24/24 |
| reported / controls | 24/24, clean | 24/24, clean |
| turns | 582 | 634 (+9%) |
| cost (USD) | 0.711 | 0.750 (+5.5%) |
| probe executed | n/a | in >=9 of 14 runs |

Transcripts (arm B, /tmp/surface-ab/wbase/tx/): the model reads whole files with `cat -n`, greps for the
absent guard (`strings.EqualFold` beside the README's "the comparison is exact") and diffs every finding
against the README contract; one run held a 3,964-line module in ~57k input tokens (~14 tokens/line). The
probe was decisive once (w01: its row named the unguarded thumbnail join directly), silent once (w02: 0
rows; it missed the concatenated SQL filter and the EqualFold lookup), and noisy on JavaScript (w06: 46
rows, 36 unguarded, mostly timezone/decode sinks). Reading-everything is estimated to survive to ~5-8k
source lines; beyond that the model must page, and a global inventory becomes the only map.

## Third A/B: the 8-15k-line corpus x01-x07 (#451)

Seven cases of 5,123-9,907 non-test source lines (x03 under the 8,000 floor, recorded), 37-84 files each,
same defect shape, plus a clean control. Arm A built from origin/main (skill 0.7.23, no probe); arm B from
the same tree plus the reverted step-1 instruction (skill 0.7.24, cherry-picked for the run only). Same
model, runner, 2 runs per case, concurrency 4. Arm B's first pass invalidated x04 twice because its
fixture/bin/ scripts were silently git-ignored (fixed: 5d133bed, corpus gate test bb01049e with RED
observed); x04 was re-run after the fix and its 4 defect-runs count below.

| measure | arm A (no surface) | arm B (surface) |
|---|---|---|
| reported (defect-runs) | 24/24 | 24/24 |
| caught (defect-runs) | 23/24 | 21/24 |
| x01-archive-intake (9.4k lines) | 4/4 | 2/4 |
| x03-artifact-server (unstable) | 3/4 | 3/4 |
| false positives, controls | 0, clean | 0, clean |
| turns | 695 | 650 (-6.5%) |
| cost (USD) | 0.893 | 0.762 (-15%) |

The read-everything ceiling was underestimated: at 9,907 lines the model still catches 23/24 without the
probe, by paging with grep instead of reading whole files. Where the probe instruction differed visibly it
HURT: on x01 both arm-B runs pinned the join defect the probe names and missed the unescaped-note branch it
does not, while both arm-A runs caught both. The turn/cost saving comes with the recall loss.

## Decision

The skill does not ship wired to the probe, now measured at three scales: flat on u* (36/39 both arms),
flat on w* (24/24 both arms, +9% turns), and flat-to-worse on x* (23/24 vs 21/24, with x01 losing two
defect-runs to the instruction). `tsp probe surface` ships as an opt-in tool; the step-1 instruction stays
reverted (0976ad1c). What the corpora are worth independent of the probe: 22 new cases (u01-u08, w01-w07,
x01-x07) with 45 planted defects and three clean controls, all passing the corpus gate. Remaining known
gaps, recorded not scheduled: the JS extractor's guard-shape blindness (a check on the raw text where
siblings parse — w06 D1, x06 D1), and x03's thin suite (127 test lines).
