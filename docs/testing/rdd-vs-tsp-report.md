# RDD vs TSP: report

Issue #372, milestone 7. Protocol: [`rdd-vs-tsp-protocol.md`](rdd-vs-tsp-protocol.md), committed before any
paid run, with deviations D1-D3 recorded before the analysis they affect. Analysis:
`bench/rdd-vs-tsp/analyze.py`. Every number below is that script's output on the study runs, plus per-arm
counts read from the same `result.json` files; nothing is computed by hand.

## Verdict

| Condition | Status | Verdict |
|---|---|---|
| Change (base = fixed code) | registered | **RDD better at detection** (A − B = 0.136, 95% CI [0.061, 0.212]) |
| Audit (empty base) | exploratory (D1, D2) | no verdict: RDD cannot review a whole repository; TSP alone detects 0.682 |

In plain terms: on a change that introduces a real bug, Gentle AI's native review named the bug in every run,
TSP in 86% of them, so RDD is the better detector here, at a quarter of the cost. The two are not
complementary at detection: every defect TSP detected, RDD detected too (union − max = 0). They are
complementary at what each produces: TSP leaves a failing test that pins the defect in 82% of runs and raised
no finding on clean code, while RDD pins nothing and raised 6.8 findings per clean-control run.

## Erratum (2026-11-07): the TSP arm was contaminated

After the report was merged, the TSP misses were read case by case (#385). Arm B ran Claude Code with
`--agent-config bench`, which isolated `CLAUDE_CONFIG_DIR` but kept the operator's `HOME`, so Claude Code also
loaded the operator's own skills from `~/.claude/skills` (#390). In 11 of the 66 change-condition runs the agent
loaded the operator's `branch-isolation` skill, created a separate `git worktree` as it instructs, and wrote its
tests there, where the bench never looks. Arm A was not affected: Gentle AI starts its reviewers with
`--setting-sources ""` and no tools.

| TSP, change condition | Runs | Detected | Pinned | US$ per run | Turns |
|---|---|---|---|---|---|
| Contaminated (loaded `branch-isolation`) | 11 | 4 | 4 | 0.098 | 30 |
| Clean | 55 | 53 (0.964) | 50 (0.909) | 0.050 | 23 |

In the audit condition 25 of 66 runs were contaminated (8 detected); the 41 clean ones detected 37. Four of the
six change-condition control runs were contaminated too; the two clean ones raised one finding between them.

What this changes: the registered verdict above is kept as computed, but **it does not measure the method**: arm B
was handicapped by a bench defect arm A did not have. The clean subset is not a valid replacement either: which
runs loaded the skill was the agent's choice, not random, so a rate over the clean runs is post hoc. The bench is
fixed in #390, and the comparison is rerun with the same protocol in #388; until then the defensible reading is
"RDD detects at least as well on a change, at lower cost; the size of its lead is unknown".

## Setup

- Binary: `tsp` 0.8.0 (cd14d69), built from a clean commit of `main` (`-ldflags -X buildinfo.Commit=<sha>`).
- Gentle AI: `gentle-ai` 4.3.2-0.20261106111438-92adfadf8877, native review headless, all four lenses,
  consent granted for the exact candidate. `managed_assets_outdated` worked around with
  `gentle-ai sync --agents pi,claude-code,opencode` (#369, reported upstream as gentle-ai#6015).
- Model: the `haiku` alias in both arms. RDD's reviewers request `opus`/`sonnet` per lens; the bench's
  `claude` shim rewrites the request, and every arm A run's `reviewers.jsonl` records `claude-haiku-5-5` as
  the answering model; every arm B transcript records `claude-haiku-5-5`. No run was excluded.
- Cases: the 22 seeded real Go bugs and two clean controls the protocol lists, 3 runs per arm and condition
  (72 runs per arm, 216 in total).
- Execution: each arm ran with concurrency 1, as registered; the three arms ran as parallel processes on one
  machine. Two reboots interrupted the study; it was resumed with the same binary (D3).

## Results

### Change condition (registered)

`analyze.py` output:

```
paired valid case-runs: 66 defect, 6 control; excluded 0
cases: 22, defects in valid runs: 66
detected  A (RDD) 1.000 [1.000, 1.000]
detected  B (TSP) 0.864 [0.788, 0.939]
detected  union   1.000 [1.000, 1.000]
A - B             0.136 [0.061, 0.212]
union - max(A,B)  0.000 [0.000, 0.000]
case-runs detected only by A: 9, only by B: 0
TSP pinned 54/66, evidence-backed 57/66
false positives on controls: A 6.833, B 0.000 (findings per control run)
cost per case-run US$: A 0.015, B 0.064; seconds: A 102.135, B 152.723
verdict: RDD better at detection
```

| Measure | RDD | TSP | Union |
|---|---|---|---|
| Detected (95% CI) | 1.000 [1.000, 1.000] | 0.864 [0.788, 0.939] | 1.000 |
| Pinned by a test | 0 | 54/66 (0.818) | — |
| Evidence-backed | 0 | 57/66 (0.864) | — |
| Findings per defect run | 1.7 | 1.3 | — |
| Unmatched findings per defect run | 0.0 | 0.0 | — |
| False positives per control run | 6.8 | 0.0 | — |
| Cost per case-run (US$) | 0.015 | 0.064 | — |
| Time per case-run (s) | 102 | 153 | — |

How the matcher credited the detections: RDD 65 line matches and 1 keyword match; TSP 50 line matches and 7
keyword matches. TSP's nine misses are spread over eight cases (h12 twice; r05, r08, r17, r24, r29, r34,
r37 once each), so no single case accounts for the gap.

### Audit condition (exploratory, TSP only)

Gentle AI refused every whole-repository review at START (D2), so this condition has no RDD arm and no
verdict. TSP over the same 22 cases with an empty base:

| Measure | TSP, audit | TSP, change |
|---|---|---|
| Detected | 45/66 (0.682) | 57/66 (0.864) |
| Pinned by a test | 42/66 | 54/66 |
| False positives per control run | 0.2 | 0.0 |
| Cost per case-run (US$) | 0.075 | 0.064 |

Seeing the change raises TSP's detection by 18 points: TSP uses the diff when it has one, and it still finds
two thirds of the defects without it, where RDD cannot run at all.

### Pilot (not part of either condition)

Cases h06, h20, r05, r16, r17, r27, r34, r37, one run per arm, binary d109cf9, change condition. Both arms
detected 8/8; TSP pinned 8/8. RDD US$0.013 and 74 s per case-run, TSP US$0.036 and 123 s. Both controls were
invalid (no `fix/all`), which led to D1.

## Spend

| Item | US$ |
|---|---|
| Setup runs (f01, both arms) | about 0.06 |
| Pilot | 0.39 |
| First study run, lost to a reboot that wiped `/tmp` | unknown; its logs were lost |
| Six arm B control runs lost by the first resume (D3) | about 0.55 |
| Study, change condition, arm A (RDD) | 1.06 |
| Study, change condition, arm B (TSP) | 4.59 |
| Study, audit condition, arm B (TSP) | 5.38 |
| **Recorded total** | **about 12.0**, within the US$25-40 budget |

## Exclusions and incidents

- No case-run was excluded from the final analysis.
- 22 arm A runs that stopped at `managed_assets_outdated` after a reboot, and two `result.json` files cut
  short by a reboot, were deleted and repeated (D3). The runner defect that let a stopped review look like an
  empty one is fixed by #381.
- One arm A run had a fixture suite that was not green; it was repeated.
- An analysis printed before D3 ("RDD better" on 15 cases) used an incomplete, non-random subset and is not a
  result.

## Threats to validity

- **Diff visibility (registered bias).** In the change condition the diff is the defect: each case's
  candidate differs from its base only by the bug, so a reviewer that comments on the changed lines names the
  bug. This is exactly the setting RDD is built for, and it favours RDD; the audit condition removes it but
  RDD cannot run there.
- **Controls are not like-for-like for RDD.** A clean control has no fix, so it is presented over an empty
  base: for RDD that is a review of the whole fixture, which is the audit setting, and most of its 6.8
  findings per run are claims about code with no defect. On a real change with no bug the false-positive rate
  was not measured.
- **Matcher, not a judge.** Both arms are scored by the bench's deterministic plan matcher (file at the
  defect's line within tolerance, or a defect keyword). It cannot tell a correct explanation from a lucky
  citation of the right line; it judges both arms the same way. With the diff equal to the bug, a line match
  for RDD mostly means "commented on the change".
- **One model.** Haiku in both arms, method against method. RDD as shipped uses stronger models per lens;
  this study does not measure that configuration.
- **Corpus.** 22 Go bugs from public repositories, one keyed defect each; other languages and multi-defect
  changes are not measured.

## What this means for using TSP with RDD

**RDD is the better and cheaper detector on a change.** When the question is "does this diff introduce a
bug?", the native review named it in every run at US$0.015, against TSP's 86% at US$0.064. TSP should not be
sold as a better reviewer of changes; on this evidence it is not.

**TSP's value is what RDD does not produce.** In 82% of runs TSP left a test that fails on the defect and
passes on the fix, so the bug is guarded from then on; RDD's finding is prose that someone still has to turn
into a fix and a test. TSP raised no finding on clean code, where RDD raised several per run, and TSP still
found two thirds of the defects with no diff to read, where RDD cannot review at all.

**So they are complementary in role, not in detection.** The evidence supports running RDD to find and TSP to
pin: RDD on every high-risk change, TSP to turn what RDD reports, or what a change touches, into failing
tests. Whether TSP given RDD's findings pins more defects than either alone is the next measurement; this
study did not test it.
