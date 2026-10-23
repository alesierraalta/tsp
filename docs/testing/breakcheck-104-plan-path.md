# Breakcheck readiness report

## Candidate identity

- Project/root: `rdd-plus` at worktree `../rdd-plus-worktrees/104-declared-plan-path`
- Execution label: `breakcheck-104-plan-path-2026-09-18`
- Candidate: the working tree of `fix/104-declared-plan-path` — base `90ab13b`, six modified files (`internal/bench/score.go`, `adjudication.go`, `run.go`, `score_test.go`, `linked_test.go`, `bench/README.md`), uncommitted at campaign time.
- Scope: the plan-path resolution the change introduces and the readers that consume it. Not claimed: the corpus, the scorer's matching rules, `rescore`, or any other bench behaviour.
- Base: `90ab13b`. HEAD: `90ab13b` plus the working-tree changes.
- Environment and isolation: a throwaway copy of the worktree under `/tmp`; probes run as Go tests over `t.TempDir()` workspaces. No model call, no bench run, no writes to the repository.
- Verifier evidence: `assess` returned high risk (`shell_process` signal on `internal/bench/run.go`) and an independent verifier was launched; its verdict is separate from this campaign.

## Budget

- Probe budget: 5 probes.
- Time budget: 20 minutes wall clock.
- Stop condition: five probes or 20 minutes, whichever first. Five ran.

## Risks and probes

| Risk | Probe | Expectation | Observed | Evidence |
|---|---|---|---|---|
| Declaration escapes the worktree | P1: `.rdd-plus.json` declares `../escape.md`, default plan present | refusal named, fallback to the default path, default plan read | `plan_found=true plan_path="docs/testing/test-plan.md" format="table" rows=1 notes=[declared plan path refused (.rdd-plus.json escapes the worktree: "../escape.md"); read docs/testing/test-plan.md instead]` | probe output P1 |
| Declaration is absolute | P2: declares `/tmp/outside-plan.md`, default present | refusal named, fallback | `plan_found=true plan_path="docs/testing/test-plan.md" notes=[… must be repository-relative: "/tmp/outside-plan.md" is absolute …]` | probe output P2 |
| Declared file absent while the default plan exists | P3: declares `docs/testing/test-plan-other.md`, writes the default plan instead | `plan_found=false` (the declaration is the run's promise) but the artifact should let a reader tell the two apart | `plan_found=false plan_path="" format="empty" rows=0 notes=[no plan]` — the note says nothing about the declaration | **F-104-1** |
| Symlink inside the workspace points outside | P4: declares `docs/testing/link.md`, a symlink to a file in another temp dir | with lexical-only validation, the outside file is read | `plan_found=true plan_path="docs/testing/link.md" notes=[]` — read through the symlink | **F-104-2** |
| `plan_path` leaks an absolute path or is set without a plan | P5: declares `docs/testing/deep/plan.md` and writes it | `plan_path` is workspace-relative and set only when found | `plan_path="docs/testing/deep/plan.md"`; P3 shows it empty when nothing was read | probe output P5 |

## Findings

| ID | Severity by consequence | Evidence and smallest reproducer | Disposition |
|---|---|---|---|
| F-104-1 | limited — diagnosability, not a wrong score: a workspace that declared a path and wrote its plan elsewhere is reported as a plain `no plan`, the exact ambiguity #104 set out to remove, in a case the change does not cover | P3 above: declare `docs/testing/test-plan-other.md`, write `docs/testing/test-plan.md`; observed `plan_found=false`, `notes=[no plan]` | FAIL (limited) — the documentation half was fixed in this PR: `bench/README.md` now says "selected plan file is absent" and spells out the declared-else-default rule, and the `NoPlan` comment no longer claims the fixed path. The artifact half stays open: `result.json` still does not name the declaration it overrode. |
| F-104-2 | limited — a workspace symlink lets the scorer read a plan outside the workspace; `plan.ResolvePath` validates lexically | P4 above: symlink `docs/testing/link.md` to a file in another directory; observed `plan_found=true` | FAIL (limited) |

Both existed before this change: on master the declared path was never read at all, so F-104-1's symptom was broader and F-104-2's symlink could be followed at the default path too. Neither is caused by the candidate.

The independent verifier, run after this campaign, confirmed the functional result and reached the same artifact reading with its own reproduction, and separately flagged the wording this report had missed: `bench/README.md` said "no plan anywhere", which overstates the selected-path semantics. That half of F-104-1 is fixed in this PR.

No other suspicion survived its rerun: the refusal notes are specific and name both the problem and the fallback, `plan_path` never leaks an absolute path, and the fallback reads the default plan correctly.

## Omissions

| Risk omitted | Reason | Residual risk |
|---|---|---|
| The kept copy and `rescore` end to end | Budget; the copy's name is pinned by a test and the change keeps it | A run whose declared plan is kept may still be rescored from the copy without noticing the declaration; unmeasured |
| `plan admit`/`check` interplay with a declared path | Out of the change's blast radius | None identified |
| Race conditions, concurrency, interrupted writes | Not applicable to a read-only resolution over one file | None identified |
| A declaration that is valid JSON but holds an unexpected type | Covered by `plan.ResolvePath`'s own tests, not re-run here | Low |

## Readiness disposition

- Status: `PASS` with two limited follow-ups.
- Rationale: the change does what it claims — the resolved path is honoured, a refusal falls back with a specific note, and `plan_path` records what was read. Neither finding is caused by the candidate; both are pre-existing and limited, and one of them (F-104-1) is squarely in the dimension this change improves but does not close, so it is recorded as a follow-up rather than a blocker.
- Accepted risks: F-104-1 and F-104-2 remain open, with their reproducers above.
- Accepting authority: the maintainer (this run records them; it does not accept them silently).
- Residual risk: a reader still cannot distinguish "declared elsewhere and missing" from "nothing at all"; and a symlinked declaration can point the scorer at a file outside the workspace.
