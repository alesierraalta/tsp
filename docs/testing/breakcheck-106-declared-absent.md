# Breakcheck readiness report

## Candidate identity

- Project/root: `rdd-plus` at worktree `../rdd-plus-worktrees/106-declared-absent-note`
- Execution label: `breakcheck-106-declared-absent-2026-09-18`
- Candidate: the working tree of `fix/106-declared-absent-note` — base `118de15`, four modified files (`internal/bench/score.go`, `adjudication.go`, `score_test.go`, `bench/README.md`), uncommitted at campaign time.
- Scope: the missing-plan note the change introduces, its interaction with the refusal note and with the two scoring functions. Not claimed: scoring, matching, the corpus, `rescore`, or the kept copy.
- Environment and isolation: a throwaway copy of the worktree under `/tmp`; probes run as Go tests over `t.TempDir()` workspaces. No model call, no bench run, no writes to the repository.
- Verifier evidence: none yet at campaign time; independent verification follows separately.

## Budget

- Probe budget: 5 probes. Time budget: 20 minutes. Five ran.

## Risks and probes

| Risk | Probe | Expectation | Observed | Evidence |
|---|---|---|---|---|
| The new note duplicates or contradicts the refusal note | P2: declaration `../escape.md` with a plan at the default path | exactly one note, the refusal, no absence note | `found=true notes=1 ["declared plan path refused (… escapes the worktree: \"../escape.md\"); read … instead"]` | probe P2 |
| The note fires when it should not | P5: no declaration, no plan | the historical single `no plan` note | `notes=["no plan"]` | probe P5 |
| The note fires twice for one missing plan | P1: declared absent, default plan present | exactly two notes, both specific | `notes=["no plan", "declared plan \"…test-plan-other.md\" was not found; default plan \"…test-plan.md\" was not read"]` | probe P1 |
| **A declared path that exists but is not a readable plan** | P3: declared file `chmod 000`; P4: declared path is a directory | the note must not claim the path is missing | **both cases printed `was not found` about a path that exists** | **F-106-1** |
| The two scorers agree on an unreadable plan | P3/P4 repeated through `ScoreWorkspaceWithAdjudication` | same outcome as the plain scorer | the plain scorer reports the plan absent; the adjudicated one refuses with `read plan: … is a directory` | **F-106-2** |

## Findings

| ID | Severity by consequence | Evidence and smallest reproducer | Disposition |
|---|---|---|---|
| F-106-1 | material — a false statement in the run's record: the note said `was not found` about a path that exists, which is exactly the kind of claim this change was written to stop | P3 and P4 above: declare `docs/testing/test-plan-other.md` and make it unreadable (`chmod 000`), or declare a path that is a directory; observed note `declared plan "…" was not found` | **FIXED in this PR** — `noteMissingDeclaredPlan` now distinguishes an absent path from one that exists and was not read (`pathExists`), the note reads `exists but was not read as a plan`, and `TestScoreWorkspaceOnADeclaredPathThatCannotBeReadAsAPlan` pins it. The writer's tests covered absent files only; this case came from the campaign. |
| F-106-2 | limited — the two scoring functions disagree: `ScorePlanFile` discards a non-`ErrNotExist` read error, so the plain scorer treats an unreadable plan as absent while `ScoreWorkspaceWithAdjudication` refuses and returns the error | same reproducer, scored through both functions | FAIL (limited) — pre-existing, not introduced here, and out of this change's blast radius. Recorded for a follow-up issue: either the plain scorer should surface the read failure, or the asymmetry should be stated where the two are documented. |

No other suspicion survived its rerun.

## Omissions

| Risk omitted | Reason | Residual risk |
|---|---|---|
| The `chmod 000` case as a pinned test | It only reproduces for a non-root user, so a suite run as root would read the file and the test would flake | The directory case pins the same code path deterministically; the unreadable-file case is probed here and recorded |
| The kept copy and `rescore` with a declared-absent plan | Budget; the copy path is unchanged by this note | A rescore of such a run reads nothing, as before |
| Notes appearing twice when a caller scores the same workspace twice | Out of scope: each call builds its own `Result` | Low |

## Readiness disposition

- Status: `PASS`, with F-106-2 as an open limited follow-up.
- Rationale: the change does what #106 asks — it names the declaration a missing plan overrode and what the default path holds — and the campaign's material finding was fixed and pinned before the PR, so no false claim remains in the record.
- Accepted risks: F-106-2 remains open with its reproducer; the unreadable-file case is recorded rather than pinned.
- Accepting authority: the maintainer (recorded, not silently accepted).
- Residual risk: an unreadable declared plan is still scored as absent by the plain scorer, now with a truthful note and a documented asymmetry.
