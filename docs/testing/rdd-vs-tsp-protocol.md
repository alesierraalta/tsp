# RDD vs TSP: pre-registered protocol

Milestone 7, issue #372. This file is committed before any paid run of the study; the decision rule below
is fixed here and is not changed after the numbers are seen. A deviation is recorded at the end of this
file, with its reason, before the run it affects.

## Question

Is TSP (the `test-strategy` skill driving `tsp`) better than, complementary to, or equivalent to Gentle AI's
RDD native review at finding defects in a change?

## Arms

Both arms read the same repository: `tsp bench run --as-change` presents every case as a base commit holding
the fixed code (fixture plus `fix/all`) and a candidate commit holding the fixture with its defect.

| Arm | Command |
|---|---|
| A, RDD | `tsp bench run --runner gentle-review --as-change --model haiku` |
| B, TSP | `tsp bench run --runner claude --agent-config bench --as-change --model haiku` |

Same model for both: the `haiku` alias. Arm A's reviewers are forced onto it by the bench's `claude` shim,
and `reviewers.jsonl` records the model that answered (`claude-haiku-5-5` in the setup runs). Arm B's
Claude Code stream records its model too. A run whose answering model is not `claude-haiku-5-5` is invalid.

Gentle AI review runs headless with consent granted for the exact candidate and all four lenses. `gentle-ai
sync --agents pi,claude-code,opencode` is the documented workaround that keeps `review status` able to
offer START (#369).

## Cases

Selected before any study run with `random.Random(372).sample(sorted(h* and r* case names), 22)`. All 22
are real upstream Go bugs with one keyed defect each (22 defects).

Study: h04-testify-notcontains-noncontainer-message, h06-ent-like-pattern-escape,
h10-gokit-generic-atomic-alignment, h12-gomicro-plan-delegate-notify-recovery,
h20-chi-request-pattern-subrouter, h22-beszel-battery-name-invalid-utf8,
h28-zap-atomiclevel-form-content-type, r01-age-bech32-unicode, r05-validator-uuid-case,
r08-age-reader-eof, r11-caddy-port-range, r12-memos-webhook-unspecified, r16-gin-upload-existing-dir,
r17-echo-gzip-encoding-case, r24-compose-relay-already-connected, r25-asdf-git-update-diverged,
r27-esbuild-mangle-label-try-loop, r29-jaeger-mcp-search-depth-unlimited, r33-zap-json-no-level-encoder,
r34-stringx-substr-reversed-range, r37-filebrowser-directory-upload-hook,
r38-jaeger-criticalpath-inclusive-child.

Clean controls (false positives only): c01-clean-allocate, g05-clean-backoff.

Pilot, `random.Random(3720).sample(study, 8)`: h06, h20, r05, r16, r17, r27, r34, r37, plus both controls,
one run per arm. The pilot validates the setup; its numbers are not part of the study and are reported
apart.

## Runs

Study: 22 cases plus 2 controls, 3 runs per arm (144 case-runs). Concurrency 1, `--max-cost-usd 20` per arm,
binary built from a clean commit of main. A run that fails for infrastructure (rate limit, crash, timeout)
is retried once by the bench; a run still failed is invalid and excluded from both arms for that case and
run index, so arms stay paired.

## Measures

Per arm, over valid case-runs:

- **Detected**: the defect is named by a claim, scored by the bench's plan matcher (`Result.Detected`):
  the claim cites the defect's file at its line within the bench's line tolerance, or names one of its
  keywords. Arm A's claims are its review findings (lenses deduplicated by location); arm B's are its plan
  Findings rows. The same matcher judges both arms.
- **Pinned** (arm B only): a test the run left fails on the defect and passes on the fix (`Result.Caught`).
- **Evidence-backed** (arm B only): the finding links observed ledger evidence (`Result.Found`).
- **False positives**: findings on a clean control (`UnmatchedFindings` on c01 and g05).
- **Union**: for each case and run index, the defect counts as detected when either arm detected it.
- **Cost** (US$) and **time** (seconds) per case-run.

Rates are detected defects over defects in valid case-runs. 95% intervals come from a bootstrap over cases
(10,000 resamples, seed 372), resampling whole cases with all their runs.

## Decision rule

Let A, B and U be the detection rates of RDD, TSP and their union, and M = max(A, B).

1. **One arm is better** when the 95% interval of the paired difference A − B excludes 0. The arm with
   the higher rate is better at detection.
2. **Complementary** when U − M ≥ 0.10 and the 95% interval of U − M excludes 0: each arm detects defects
   the other misses, in a share worth running both.
3. **Equivalent** when neither 1 nor 2 holds: at this sample size the data does not separate them.

1 and 2 can hold together (one is better and the other still adds a share). TSP's pinning is reported
beside the verdict, never folded into detection: a pinned defect is one a test now guards, which a review
cannot give, and the reader weighs that against cost.

## Known bias

Reviewing a regression diff shows the reviewer exactly which lines changed, and on most cases the removed
guard is visible in the diff. Arm B gets the same history but may not read it. This is the setting each tool
is used in (RDD reviews a change; TSP is run on a change), so it is kept, and it is named here because it
favours arm A on cases whose fix is a small, visible guard.

## Analysis

`bench/rdd-vs-tsp/analyze.py <arm A results dir> <arm B results dir>` reads both arms' `result.json` files
and prints every measure above with its interval and the verdict the rule gives. It uses only the Python
standard library.

## Deviations

**D1 (2026-11-06, after the pilot, before any study run).** The pilot (h06, h20, r05, r16, r17, r27, r34, r37,
one run per arm, binary d109cf9) validated the setup and showed two things.

1. Both clean controls were invalid in both arms: they have no `fix/all`, so `--as-change` could not build a
   base. Controls are now presented over an empty base (`ScaffoldAsChangeFrom`), so the whole fixture is the
   change and every finding on it is a false positive. The registered study is otherwise unchanged.
2. Both arms detected 8/8, every detection a line match (one TSP keyword match), at US$0.013 (RDD) and
   US$0.036 (TSP) per case-run. A ceiling this close to 1 can only answer "equivalent", because the regression
   diff shows the removed guard (the known bias above). A second condition is added and labelled
   **exploratory**: the same 22 cases and controls with `--as-change --empty-base`, so the defect is hidden in
   the whole code instead of shown as the diff. Same arms, model, runs and analysis; it is reported apart from
   the registered result and its verdict does not replace the registered one.

Pilot numbers are reported apart and are not part of either condition.

**D2 (2026-11-06, during the study, before any analysis).** In the audit condition Gentle AI refuses every
review at START: the whole repository as a change exceeds the evidence budget of a lens ("candidate
evidence is never truncated and retrying this exact candidate cannot succeed, so split it into a chained
sequence of smaller reviewable commits"). 27 of the first 28 audit case-runs of arm A failed that way (the
other had a fixture suite that was not green), at no model cost. RDD reviews changes and cannot audit a
whole codebase by design, so arm A of the audit condition was stopped. The audit condition keeps arm B
alone, read against arm B of the change condition (how much TSP depends on seeing the diff); it has no
RDD comparison and no verdict. The registered change condition is unaffected.

**D3 (2026-11-07, after the arms first finished, before the final analysis).** Two machine reboots interrupted
the study; it was resumed with `.runs/rdd-vs-tsp/resume.sh`, which keeps finished runs and reruns only
missing ones, with the same binary. Two defects of the setup were found and corrected, neither of which
changes a measure or the decision rule:

1. After the second reboot a full `gentle-ai sync` brought back `managed_assets_outdated`; 22 arm A runs
   stopped before any reviewer and the runner returned them as reviews with no findings (bench defect #381).
   The analysis excluded them already, because their answering model was unknown. They were deleted, the
   workaround was reapplied, resume now checks that review START is offered before every arm A case, and
   the runs were repeated. Two `result.json` files cut short by the reboot were deleted and repeated too.
2. The first resume did not carry arm B's clean-control runs into the arm (control case directories have no
   hash suffix); those six runs were lost and repeated.

The first analysis printed before these corrections ("RDD better at detection" on 15 cases) used an
incomplete, non-random subset and is not a result.

**D4 (2026-11-07, before the rerun of #388).** After the report, arm B was found contaminated: the claude runner
kept the operator's `HOME`, so 11 of 66 change-condition runs loaded the operator's `branch-isolation` skill and
wrote their tests in a worktree the bench never reads (#390, erratum in the report). The rerun keeps everything
registered above (cases, controls, model, 3 runs, measures, analysis, decision rule) and changes only:

1. Arm B is rerun in both conditions with a `tsp` binary built from a clean `main` that includes #391 (claude
   runs get a HOME of their own) and #389 (`tsp plan check --report`, and the `test-strategy` 0.7.17 instruction
   to record every reported defect and run that check). The rerun therefore measures TSP as it ships after the
   recording fix, which is the question #388 asks.
2. Arm A is not rerun: its reviewers run with `--setting-sources ""` and no tools, so the contamination did not
   reach them. Its 72 change-condition runs from the first study are reused unchanged.
3. Each rerun case-run is checked for contamination before the analysis: a transcript that names any operator
   skill other than `test-strategy`, or a run whose tests sit outside the workspace, is reported and excluded
   from both arms for that case and run index.

The first study's TSP numbers are reported beside the rerun as "before"; the verdict is the rerun's.

**D5 (2026-11-07, before the rerun is relaunched).** The first rerun attempt was stopped after 11 case-runs:
transcripts still referred to the operator's branch-isolation rule. Claude Code loads every ancestor's
`CLAUDE.md`, so a workspace under the operator's home received the operator's 754-line `~/.claude/CLAUDE.md`
whatever `HOME` said (#392). This reached every arm B run of the first study as well, not only the 11 that showed
it; the arm A reviewers run with `--setting-sources ""` and did not get it. The rerun is relaunched from scratch
with a binary that includes #393 (`--setting-sources user` for runs with their own config); the 11 stopped
case-runs are discarded and not reported. D4's contamination check is extended: a transcript that names any
operator instruction (branch-isolation, rtk, engram, codegraph) is excluded.

**D6 (2026-11-08, exploratory arm).** The maintainer replaced the model with `openai-codex/gpt-6-luna` after the
Claude Code session expired mid-study (`OAuth session expired and could not be refreshed`), which invalidated 74
arm B case-runs of the first rerun attempt. RDD cannot run on Luna (gentle-ai reviews through Claude Code, whose
models are Claude's), so the rerun runs **arm B only**, in both conditions, with `--runner pi --model
openai-codex/gpt-6-luna` on a binary from `main` at 3135e1b. It is reported as an exploratory arm with no verdict
against arm A; the registered comparison needs the Claude session and is not made here.

**D7 (2026-11-08, TSP only).** A third run of the TSP arm, on `openai-codex/gpt-6-luna`, with the runner-side
record nudge (#398, `--record-nudge`): one follow-up asking the run to record what it found when it changed a test
and the plan held no finding. Same 22 cases, two controls, 3 runs, same analysis; no RDD arm (Claude-only). Its
result appears in the report as "Addendum 2" and is not a verdict against arm A.
