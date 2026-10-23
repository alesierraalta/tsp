# Breakcheck readiness report

## Candidate identity

- Project/root: `rdd-plus` at worktree `../rdd-plus-worktrees/feedback-record-fidelity`
- Execution label: `breakcheck-feedback-fidelity-2026-09-18`
- Candidate: base `b5678e2`. Seven files: `cmd/rdd-plus/main.go`, `cmd/rdd-plus/main_test.go`, `internal/feedback/feedback.go`, `internal/feedback/feedback_test.go`, `README.md`, `assets/skills/test-strategy/SKILL.md`, `assets/skills/breakcheck/SKILL.md`. Uncommitted at campaign time.
- Scope: where the feedback record lands and what its `skill` field says. Not claimed: the verdict vocabulary, the aggregate, the bench's own provenance, or the rows already written.
- Environment and isolation: throwaway config directories under `/tmp`, one built binary from this tree, and the real ledger read only to count its lines. No model call, no repository writes.

## Budget

- Probe budget: 5 probes; six ran (the sixth checked the documented resolution order). Time budget: 20 minutes.

## Risks and probes

| Risk | Probe | Expectation | Observed | Evidence |
|---|---|---|---|---|
| **The claim this change exists for**: a run whose runner set the config variable still writes into the operator's ledger | P1: build the CLI, point `CLAUDE_CONFIG_DIR` at a throwaway directory, submit a filled template | the record lands in the throwaway directory and the operator's ledger does not grow | `ledger real: sin filas nuevas` (178 en ese momento; 179 cuando el verificador lo reprodujo, y en ambos casos **cero filas añadidas por la sonda**); `config aislado: 1 fila(s)`; readback from that directory prints the report | probe P1 |
| Precedence between the two variables is not what the README says | P2: both set, one holding the report | `CLAUDE_CONFIG_DIR` wins | the summary read the report from the `CLAUDE_CONFIG_DIR` directory | probe P2 |
| The new shape check refuses a legitimate report | P5: eight shapes submitted through the CLI | the four valid ones recorded, the four invalid ones refused with the shape named | `breakcheck 0.1.1`, `test-strategy 0.3.10`, `unknown`, `alpha` → recorded; `0.3.10`, prose, `Test-Strategy 0.3.10`, `breakcheck 1` → `rc=2` with `skill "…" must match <name> or <name> <version>` | probe P5 |
| Old rows stop loading once the shape is enforced | P6: a hand-written legacy row with `skill: 0.3.6` | still counted and printed | `run feedback: 1 report(s)` … `0.3.6: 1 (paid 1, partly 0, ceremony 0)` | probe P6 |
| The template's identity is itself invalid | P1's template line | matches the shape | `skill: test-strategy 0.3.10` | probe P1 |
| The documented resolution order contradicts the code | P1/P2 against the README wording | same order | README states `CLAUDE_CONFIG_DIR`, then `PI_CODING_AGENT_DIR`, then `~/.claude`, empty ignored | probe P1/P2 |

The four ❌ marks in the probe output are a defect in the probe's own string comparison (`RECHAZA(rc=2)` against `RECHAZA`), not in the candidate: every refusal printed the expected message and exit code.

## Findings

The independent verifier reproduced the load-bearing probe and the shape boundary, corrected one thing this report got wrong (an absolute ledger count that ages: the probe added no rows at 178 and added none at 179 either, so the delta is the claim, not the number) and found one further defect in a file this change touches: the skill's own prose.

| ID | Severity by consequence | Evidence and smallest reproducer | Disposition |
|---|---|---|---|
| F-114-1 | limited — behavioural, and the point of the change: an operator who has been running under Pi with `PI_CODING_AGENT_DIR` set now reads a different ledger, so a history written under `~/.claude` looks empty until it is named | set `PI_CODING_AGENT_DIR` to an empty directory and run `rdd-plus feedback`: `no reports yet` while the operator's ledger (179 rows when the verifier reproduced it) holds the history | ACCEPTED with documentation: the README states the resolution order **and** that a ledger under another directory is reached with `--config-dir`. Fixing it in code would mean guessing which ledger the operator meant |
| The skill's own prose named `rdd-plus 0.3.7` while its frontmatter requires 0.3.8 and the build is 0.3.8 | low | found by the independent verifier at `assets/skills/test-strategy/SKILL.md:26`; the sentence contradicted the field a test pins | FIXED in this PR: the prose now names 0.3.8 |
| F-114-2 | limited — the shape is enforced on the CLI write path only: `feedback.Record()` still stores whatever a Go caller passes | call `Record` with `Skill: "0.3.6"`; the row lands unchanged | ACCEPTED, recorded: `Record` is the internal API the tests use, and the CLI is the only writer in production. P6 shows the read side is deliberately tolerant |
| The README's own example was stale | low | the example named `breakcheck 0.1.0` while this change bumps the skill to `0.1.1` | FIXED in this PR: the example now says `<its version>` instead of a number the next bump would falsify again |

## Omissions

| Risk omitted | Reason | Residual risk |
|---|---|---|
| Migrating the 137 bench rows already in the operator's ledger | The ledger is append-only; rewriting history to look tidy contradicts what it is for | The old noise stays visible in the by-project readback; new runs no longer add to it |
| A bench run end to end after this change | It spawns agents and costs money; P1 reproduces the mechanism the bench relies on (`internal/bench/env.go` sets the same variable) without an agent | A runner that sets neither variable would still write to `~/.claude` — which is the operator's ledger by definition |
| Shape rules for the other fields | Out of scope | `ts`, `repo` and `verdict` keep their existing validation |

## Readiness disposition

- Status: `PASS`, with two accepted limited findings and one low-value fix applied.
- Rationale: the load-bearing claim is measured end to end — a run whose config variable is set no longer grows the operator's ledger — the shape check accepts every legitimate form and refuses the four that made the field incomparable, and legacy rows keep loading. Both findings are documented rather than hidden.
- Accepted risks: F-114-1 and F-114-2 above; the pre-existing bench rows stay.
- Accepting authority: the maintainer.
- Residual risk: an operator with several config directories can read the wrong ledger until they pass `--config-dir`, which the README now says.
