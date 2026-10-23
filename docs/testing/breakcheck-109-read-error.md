# Breakcheck readiness report

## Candidate identity

- Project/root: `rdd-plus` at worktree `../rdd-plus-worktrees/109-scorer-read-error`
- Execution label: `breakcheck-109-read-error-2026-09-18`
- Candidate: the working tree of `fix/109-scorer-read-error` — base `cca8254`, five tracked modified files (`internal/bench/adjudication.go`, `score.go`, `score_test.go`, `bench/README.md`, `docs/testing/dogfooding-log.md`) plus this report. Uncommitted at campaign time. The change carries two things: the #109 read-failure note, and the repair of the guard `master` lost when #107's commit omitted `adjudication.go`.
- Scope: the note a failed plan read produces, its interaction with the #106 declaration note, and the refusal contract shared by the two scorers. Not claimed: scoring, matching, the corpus, or `rescore`'s own resolution logic.
- Environment and isolation: a throwaway copy under `/tmp`; probes run as Go tests over `t.TempDir()` workspaces, as uid 1000 so a `chmod 000` file is genuinely unreadable. No model call, no bench run, no writes to the repository.

## Budget

- Probe budget: 5 probes; six ran (the sixth checked `rescore`'s loud failure). Time budget: 20 minutes.

## Risks and probes

| Risk | Probe | Expectation | Observed | Evidence |
|---|---|---|---|---|
| The plain scorer still calls an unreadable plan absent | P1: the default path is a directory, no declaration | both scorers name the failure; only the adjudicated one also returns an error | plain `["no plan", "plan read failure: … is a directory"]`; adjudicated same notes + error | probe P1 |
| Two notes describe the same unreadable file | P2: the declared path is a directory | the #106 declaration note stands down | `["no plan", "plan read failure: …"]` — no declaration note | probe P2 |
| An absent plan starts carrying read-failure wording | P3: declared path absent, default absent | the #106 notes exactly | `["no plan", "declared plan \"docs/testing/missing.md\" was not found; default plan … holds nothing either"]` | probe P3 |
| The failure note is vague | P4: the default plan exists but is `chmod 000` | the note names the cause | `"plan read failure: open …: permission denied"` | probe P4 |
| **The repair actually restores the refusal contract** | P5: a declared path symlinked outside | both scorers refuse identically, neither reads it | plain and adjudicated: `found=false path=""`, the same refusal note, no error | probe P5 |
| An unreadable kept copy is scored in silence | P6: `ScorePlanFileWithAdjudication` over a directory | loud failure | `read plan: read …: is a directory` | probe P6 |

## Findings

None. Within this scope every probe matched its expectation, including the one that mattered most: P5 shows the two scorers now refuse a path that resolves outside the workspace in exactly the same way, which is the behaviour `master` was missing.

The defects this change repairs were **not found by this campaign**: the red suite and the missing guard were surfaced by the implementation writer's own test run and by the parent's investigation, not by adversarial probing. Recorded in `docs/testing/dogfooding-log.md` as a campaign that found nothing.

## Omissions

| Risk omitted | Reason | Residual risk |
|---|---|---|
| A filesystem whose read error is neither permission nor directory | The two cases are the ones a bench workspace can produce | An exotic error would still be named verbatim by the note, so it stays truthful |
| `rescore` end to end over an old results directory | Budget; the kept-copy read is the same function probed in P6 | An old kept copy that exists but is unreadable makes rescore fail loudly, which is the intended direction |
| Concurrency, partial writes | Not applicable to a single read of one file | None identified |

## Readiness disposition

- Status: `PASS`, with no open finding.
- Rationale: the artifact now names a read failure in both scorers, the declaration note no longer repeats it, an absent plan is unchanged, and the refusal contract the previous PR lost is restored and verified live. The suite is green, which is this change's acceptance: it was red on `master`.
- Accepted risks: the omissions above.
- Accepting authority: the maintainer.
- Residual risk: none specific to this change beyond the platform-dependent wording of the underlying error.
