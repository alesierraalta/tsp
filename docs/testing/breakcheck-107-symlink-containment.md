# Breakcheck readiness report

## Candidate identity

- Project/root: `rdd-plus` at worktree `../rdd-plus-worktrees/107-symlink-containment`
- Execution label: `breakcheck-107-symlink-containment-2026-09-18`
- Candidate: the working tree of `fix/107-symlink-containment` — base `7d10868`. Six source files plus two documents: `internal/bench/score.go`, `adjudication.go`, `run.go`, `linked_test.go`, `score_test.go`, `bench/README.md`, this report and `docs/testing/dogfooding-log.md`. Uncommitted at campaign time; the campaign itself ran against the four files the change then held, before the fixes the verification rounds produced.
- Scope: the containment guard the change introduces and the resolution paths it now refuses. Not claimed: scoring, matching, the corpus, `rescore`, or the corpus scaffolding.
- Environment and isolation: a throwaway copy of the worktree under `/tmp`; probes run as Go tests over `t.TempDir()` workspaces. No model call, no bench run, no writes to the repository.
- Verifier evidence: none at campaign time; independent verification follows separately.

## Budget

- Probe budget: 5 probes. Time budget: 20 minutes. Five ran.

## Risks and probes

| Risk | Probe | Expectation | Observed | Evidence |
|---|---|---|---|---|
| A chain of symlinks hides the escape | P1: `link.md` → `link2.md` → file in another directory | refused, note names the real target | `found=false notes=["selected plan path \"docs/testing/link.md\" refused: resolves outside workspace to \"…/plan2.md\""]` | probe P1 |
| **A symlink that cannot be followed at all** | P2: `a.md` → `b.md` → `a.md` (a loop) | not read; must not be reported as a missing plan | **the loop fell through to the absent flow and the note claimed `was not found` about a path that exists** | **F-107-1** |
| The guard breaks a legitimate workspace | P3: the workspace itself is reached through a symlink | the plan is read normally | `found=true path="docs/testing/test-plan.md" notes=[]` | probe P3 |
| The guard breaks legitimate in-workspace links | P4: a directory inside the workspace is a symlink to another directory inside it | the plan is read normally | `found=true path="docs/testing/plan.md" notes=[]` | probe P4 |
| The guard refuses a plain plan | P5: no symlinks anywhere | read normally, no notes | `found=true notes=0` | probe P5 |

## Findings

| ID | Severity by consequence | Evidence and smallest reproducer | Disposition |
|---|---|---|---|
| F-107-1 | material — a false statement in the run's record, and the same class of claim this bench removed in #106: the note said `was not found` about a declared path that exists as a symlink loop | P2 above: declare `docs/testing/a.md`, symlink it to `b.md` and `b.md` back to `a.md`; observed `notes=["no plan", "declared plan \"docs/testing/a.md\" was not found; default plan \"docs/testing/test-plan.md\" holds nothing either"]` | **FIXED in this PR** — the resolution now treats a non-`ErrNotExist` failure of `EvalSymlinks` as a refusal with a note naming the failure, instead of returning the path to the absent flow. `TestScoreWorkspaceRefusesASymlinkItCannotFollow` pins it for both scorers. |

No other suspicion survived its rerun: chains resolve to the real target before the containment check, a workspace reached through a symlink is not refused, and in-workspace links keep working.

## Post-campaign verification

The independent verifier confirmed the campaign's finding and the fixes, and then found **three holes the campaign
missed** — all closed before the PR:

- **The guard skipped silently when the workspace root could not be resolved** (`rootErr` returned early), so a
  path the bench could not check was read unchecked. It now refuses with a note naming the failure.
- **The kept copy still read a refused path**: `finish` ignored the resolution and copied whatever was at the
  selected path into the results directory. It now skips the copy when the selection is refused, so a refused
  plan never reaches disk, and `rescore` has nothing to read through its own unguarded path for a newly refused
  run (an old or hand-made results directory still can: recorded as residual).
- **A rejected declaration became a way around the guard**: a config error returned before containment ran, so an
  invalid declaration plus an outside-symlink default still read that default unchecked. The fallback path is now
  checked like any other selection, and the two facts travel in one note.

The verifier also noted that the loop test's assertions are loose (it does not pin the score fields or the exact
note), while the escape table does pin them exactly. Recorded as a limit of this campaign rather than a defect:
the campaign's own claim of "no missed issues" was written before the verifier ran, and the verifier proved it
wrong. The honest score for the campaign on this change is **one material finding of its own, plus three holes it
did not see** — one of them reachable only because the change added a fallback that no probe combined with an
escaping default.

## Omissions

| Risk omitted | Reason | Residual risk |
|---|---|---|
| Windows symlink semantics | The suite already skips when symlinks are unsupported; probing Windows is out of reach here | The guard is a no-op where `EvalSymlinks` cannot fail that way, which is the safe direction |
| The kept copy and `rescore` under a refused path | Budget; the copy path is unchanged by this guard | A refused run keeps no plan copy, as a refused declaration already did |
| TOCTOU between the check and the read | The workspace is written by one agent in one run; a racing swap is out of scope for a benchmark artifact | Theoretical |

## Readiness disposition

- Status: `PASS`.
- Rationale: the guard refuses a plan that resolves outside the workspace, including through a chain, and the campaign's material finding was fixed and pinned before the PR — no note now claims a path is missing when it is there. Legitimate setups (a symlinked workspace, in-workspace links, plain files) keep working, which the probes confirmed rather than assumed.
- Accepted risks: the omissions above; no open finding.
- Accepting authority: the maintainer.
- Residual risk: a race between the containment check and the read is not addressed, and the note's OS error text varies by platform.
