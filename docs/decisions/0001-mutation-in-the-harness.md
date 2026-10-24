# 0001 — Mutation testing lives in the harness; the engine does not

Status: accepted (2026-10-23)
Task: `odd/tasks/harness-beyond-rdd.md`, T4 (spec S3)

## Context

Mutation testing answers the question `redcheck` answers for one change, but for a whole suite: if the
code is broken on purpose, does any test notice? A surviving mutant is a hole in the tests, not a bug
in the code, which is exactly the evidence TSP needs when it claims a change is tested.

The owner's inclination was to keep it inside the harness. That is right for the product and wrong
for the engine, and the difference matters:

- Mutant generation is language-specific. Go needs expression and branch operators on a typed AST;
  JavaScript needs a parser that survives decorators and JSX; Python needs one that survives
  comprehensions and decorators. Writing and maintaining three of those is weeks of work per stack,
  with no path to parity with the tools that have done it for years.
- Each mutant costs one test-suite run, so cost is dominated by the runner, not by the generator.
  Coverage-guided tools already skip tests that cannot reach the mutant.
- The mature tools expose what the harness actually needs: per-mutant status, the test that killed it,
  and machine-readable output.

Evidence gathered from the tools themselves:

The repository already does this to itself: `tools/mutants/main.go` applies a fixed list of literal
mutations to `internal/gate` one at a time and names the mutants the suite fails to kill. That is a
development check with hand-written mutants for one package, and it is worth keeping. It is not a
generator, and generalizing it into one would be a three-language engine — which is the point below.

| Stack | Tool | State | Cost control available |
| --- | --- | --- | --- |
| Go | `gremlins` | Apache-2.0, v0.6.0 (Dec 2025), 449 stars, still 0.x; its own documentation says a run on a very big module "can take hours" | coverage-guided (`--coverpkg`), file/status filters |
| JS/TS | StrykerJS | mature, designed as a CI quality gate | `coverageAnalysis: perTest` (default since v5), `--incremental` (since 6.2) |
| Python | `mutmut` 3.x | actively maintained and coverage-guided (~7–8 mutants/s in a published spike) | per-mutant selection; note that `mutmut` 2.x broke on Python 3.13, so the floor is 3.x, with `cosmic-ray` as the slower, more thorough alternative |

## Decision

The harness owns the policy, the budget, the normalization, and the gate. The mutation engine is an
external tool the harness invokes per stack:

- **Policy** — mutate only the files this change touched (the same scope `check` and `redcheck` use),
  never the whole module.
- **Budget** — a hard cap on mutants (`--max-mutants`, default small) and on wall-clock time, so a run
  is bounded and predictable. Exceeding the cap is reported, never silently reduced.
- **Invocation** — one adapter per stack: `gremlins` for Go, Stryker for JS/TS, `mutmut` for Python.
  The adapter is a runner contract, not a fork of the tool.
- **Normalization** — every mutant that survives on a changed line becomes a `findings` entry with
  `file:line`, the operator applied, and the tests that ran. This is what lets a survivor be
  adjudicated like any other finding instead of living in a separate report nobody reads.
- **Gate** — survivors above the configured threshold exit 1; a missing engine exits 2. Mutation is
  never allowed to pass silently because a tool was not installed.

The harness does not implement a mutator. `tsp mutate` is the command that owns the policy and the
report; the tools do the mutating.

## Consequences

- `tsp doctor` gains a capability line per stack, and `tsp mutate` refuses to pretend when the tool is
  absent.
- Mutation stays optional in the default flow (it is the most expensive check TSP offers) and becomes a
  sampling gate on the changed lines, not a whole-repository sweep.
- The Python adapter needs a version check, because a `mutmut` 2.x install on Python 3.13 is a silent
  source of wrong results.
- `tools/mutants` stays as TSP's own development check with its fixed literal list; the new adapters
  serve the user's repository, which TSP cannot know in advance.
- The Go adapter must sample: `gremlins` on a large module is measured in hours, and the diff-scoped
  budget is what keeps it usable.

## Alternatives rejected

- **Engine inside TSP**: weeks per language, worse mutation operators, and the harness would own bugs
  it cannot afford to maintain. The owner's instinct — inside the harness — is honored where it
  matters: the policy, the budget and the report are ours.
- **Tool only, no harness policy**: no diff scoping, no budget, no findings integration, and no exit
  code anyone can gate on. That is the state of most mutation-testing adoption, and it is why it dies
  as a habit.
