# 0002 — The kill matrix is a harness command, not a skill payload

Status: proposed (2026-11-03)
Task: #325
Follows 0001 on mutation in the harness; supersedes nothing.

## Context

`pr-review` carries a testing instrument that is not a review lens. `assets/kill-matrix.py` and
`km_cov.py` revert each production hunk of a change — or empty a new function to `return None` — run the
change's tests, and report production changes no test covers, new tests that stay green under every
reversion, tests redundant with each other, and tests that only pin a test double. Selection is by
executed lines through `sys.monitoring` (Python 3.12+); `--jobs N` gives each worker its own worktree,
database and Redis, and the installed copy already has it (`assets/kill-matrix.py:361`, with
`--worker-env` and `--worker-setup` at `:12`).

#325 proposes moving both files into `assets/skills/test-strategy/assets/` and having `pr-review`
reference them by path. Two findings stand against that as written. A payload file is prose an agent
reads: no host loader executes `assets/*.py`, and `tsp plan init` writes exactly one named template
(`internal/plan/plan.go:20`), so a move does not remove the dependency on a command — the command, not
the location, is the decision. And one copy in `test-strategy` plus a copy in `pr-review` is the
duplication #325 exists to remove, whichever of the two becomes the source of truth.

0001 already decided this shape for mutation testing: the harness owns the policy, the budget, the
normalization and the gate, and the engine is an external tool it invokes. The kill matrix is the same
shape at the scale of one change, with one difference: the engine here is a script, not a third-party
tool.

## Decision

The instrument is reachable as `tsp killmatrix`, and the harness owns the scope, the exit code and the
JSON it reads back. `pr-review` keeps the script it maintains; `tsp` invokes it by path.

- **The harness owns the contract, not the file.** `tsp killmatrix` locates the engine (an explicit
  `--instrument <path>`, then `$TSP_KILL_MATRIX`, then the installed
  `~/.claude/skills/pr-review/assets/kill-matrix.py`), refuses before it starts when the engine or a
  Python 3.12+ interpreter is missing, and maps the engine's own JSON to exit codes. Nothing is
  vendored, so the second source of truth Q2 asks about does not exist yet.
- **Surface.** `tsp killmatrix --base <ref> --src <path> (repeatable) --test-cmd <cmd>
  [--tests <path>...] [--timeout <sec>] [--jobs N] [--budget <min>] [--json <path>]
  [--instrument <path>] [--python <exe>]`.
  Exit 0: every hunk killed; 1: uncovered hunks; 2: cannot run (no engine, Python below 3.12, missing
  required flag, or the engine refused — a dirty worktree, no passing baseline); 3: the matrix did not
  finish (budget reached or `complete: false`). A partial matrix is never a pass, whatever the
  uncovered count says.
- **Scope stays the caller's.** The command never guesses which files or tests to measure: `--src` and
  `--test-cmd` are required, `--tests` is passed through. Project configuration such as
  `alterceo-pytest.sh` stays with the project, not in `tsp` and not in a payload.
- **The engine's report stays the human matrix.** Its output is streamed unchanged; the command adds
  one summary line and the exit code, and copies the JSON to `--json` when asked.
- **`pr-review` consumes the command, not a path.** Its prose keeps the severity calibration and the
  reviewing lens (`references/lenses.md` §3c); its six path references become one command reference
  once Q1 says where its source of truth lives.
- **`test-strategy` gains one sentence, no file.** The skill says what the matrix answers and that
  `tsp killmatrix` produces it. The engine is not part of a payload that hosts copy as prose.

## Why not a skill payload

- A payload file is prose an agent reads. No host loader executes `assets/*.py`, and `tsp plan init`
  writes one named template (`internal/plan/plan.go:20`), not arbitrary assets. A script that "lives in
  the payload" still needs a command to run it, so the command is the thing being decided.
- `internal/assets/assets_test.go:134` pins skill bytes by phrase. Adding a second embedded path for an
  executable turns the payload contract into a mixed bag of prose and scripts.
- One copy in the binary plus a caller in `pr-review` is one copy. One copy in `test-strategy` plus a
  copy in `pr-review` is the duplication #325 exists to remove.

## Migration

1. **Shipped as a dispatcher.** `tsp killmatrix` runs the installed engine and owns the contract. The
   embedded copy is deferred to Q2: a vendored 27.5K script that runs shell commands is a review
   surface and a second source of truth, and moving it before the owner answers buys a diff nobody
   can review in one sitting.
2. **Shipped.** The command, its exit codes, the Python floor in `--help`, and the fail-closed
   diagnostics that name the exact remedy (`--instrument`, `$TSP_KILL_MATRIX`, or install pr-review).
3. Repath `pr-review` (six references) once its source of truth is known. Until then the command
   exists and nothing calls it — a smaller inconsistency than two copies.
4. Payload version moves in the release order: embedded asset → `metadata.version` → release →
   `requires_tsp`, because `internal/assets/assets_test.go` requires `requires_tsp` to equal
   `buildinfo.Version`. This step only applies once step 1 becomes an embedded copy.

## Test

- `cmd/tsp/killmatrix_test.go` pins the contract: the exit-code table over the engine's JSON (0/1/3),
  the mapping proven end to end through a fake interpreter that writes a matrix, and the refusals that
  must happen before anything runs (missing `--base`, `--src`, `--test-cmd`, a missing engine, a Python
  below the floor) — each asserting exit 2 and the reason in stderr.
- Not yet covered: a real matrix over a fixture repository, and `--jobs 1` versus `--jobs 2` producing
  identical lists. That needs a fixture repo and a slow run, so it belongs to the live matrix in #83.

## Open questions

- **Q1** — where does `pr-review`'s source of truth live (the global skills directory, the alterCEO
  vault, or a repository), so step 3 can be a reviewable diff?
- **Q2** — is `tsp killmatrix` accepted over a payload move, or does the owner prefer the skill-owned
  copy? If the answer is an embedded copy, migration step 1 becomes a `cp` plus a `//go:embed` root.
- **Q3** — does the harness keep a compatibility shim for the old paths, or a clean cut?

## Consequences

- `tsp doctor` can report the Python floor as a capability line, like the mutation adapters in 0001.
- The instrument becomes available to any host that has the binary, not only to a machine that also
  holds `pr-review`.
- `pr-review` becomes a consumer, and its changelog stops carrying an engine.
