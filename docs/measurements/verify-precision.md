# What the blind verifier actually finds

`tsp verify` asks a second session what the first one's tests prove. This is the first measurement of
whether its answers are worth acting on, taken before any release that advertises it.

## Method

Two independent labels per candidate:

- **Mechanical label** — `tsp check --base <commit>~1`, which runs the changed tests against the parent
  commit. It answers one bounded question: does any changed test fail on the code the change started
  from? Exit 0 means at least one does; exit 1 means the tests would pass without the change.
- **Blind verdict** — `tsp verify --base <commit>~1` with a session that sees only the diff, the task,
  and the frozen read-only tree. It never sees the writer's conversation and never sees this document.

Six real commits from this repository plus one synthetic candidate whose test provably proves nothing
(a ceiling appears in `Cap`, and the test only checks a value both versions satisfy).

Model: `openai-codex/gpt-6-luna`, one run per candidate, 15-minute ceiling per session.

## Results

| candidate | what changed | mechanical label | blind verdict |
|---|---|---|---|
| `c3a52b33` | a build failure at base counts as a witness | pinned | pinned |
| `0466bcce` | `tsp mutate` accounting | pinned | insufficient |
| `1f2c9498` | `check --base` | pinned | **unpinned, 1 finding** |
| `02c74359` | host discovery and transports | pinned | pinned |
| `cfcc8075` | `tsp probe arch` | pinned | insufficient |
| `10adf451` | `tsp probe quality` | pinned | **unpinned, 1 finding** |
| `weak2` (synthetic) | a ceiling that a test never crosses | unpinned | unpinned |

Agreement: 3 of 7. Abstentions: 2. Disagreements: 2.

## The two disagreements are findings, not errors

Both rejections name a test that proves less than its commit claims, and both were checked by hand:

- `internal/check/base_test.go:8` — the check-level test injects a **fake** `RedCheck` that returns a
  preset witness, so a regression that wired `--base` to the wrong runner would still pass it. The
  range behaviour is covered indirectly by a bench test; the wiring is not covered at all.
- `cmd/tsp/quality_test.go:29` — the fixture contains a duplicated block **and** an over-complex
  function, and the test asserts only that the exit code is non-zero. Duplication alone satisfies it,
  so complexity detection can break with the suite green.

The mechanical label was right too: a changed test does fail at base in both commits. What the label
cannot see is that the failing test is not the one the commit needed. That gap is exactly what a
second session is for.

## The two abstentions are artifacts of this run

Both sessions refused because they tried to run `go test ./...` in the frozen tree and the command
**timed out after 30 seconds**, and because this run passed no `--check-output`/`--redcheck-output`,
so the artifact package said "not run". Neither is the verifier's judgement; both are the harness
handing it an incomplete package and a command budget too short to check anything.

## What this changes

1. The verifier is a source of **specific, checkable findings**, not a verdict to trust blindly. Two
   real defects out of six commits that the harness called pinned is a useful yield; a verdict alone
   is not.
2. `tsp verify` must hand the session the `check` and `redcheck` records it needs, or say plainly that
   it did not. A session that has to re-run the suite itself will abstain.
3. A session that cannot run tests must be able to say `insufficient` without that counting as a win:
   an abstention is not a clean bill of health, and the exit code already refuses to call it one.
