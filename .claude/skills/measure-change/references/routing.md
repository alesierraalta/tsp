# What reading each change owes

`SKILL.md` carries the decision. This file carries the command, the flags, and the two recipes its
Execution Steps refer to. Read the row for what you touched before you declare a metric: the instrument
decides the metric, not the other way round.

## By change class

| Touched | Command | What makes it a reading |
| --- | --- | --- |
| `cmd/`, `internal/` | `make vet && make test`; then `tsp mutate --base <ref>` | `--fail-on-survivors <n>` is the gate. Exit 2 means no engine for the stack: not a pass |
| `internal/gate` | `make mutants` | The fixed literal set M01–M23 in `tools/mutants/main.go`, against `internal/gate` only. A survivor is a missing test |
| `assets/skills/<skill>/` | `tsp bench run --cases '<set>' --runs 2 …`, then `tsp bench compare <before> <after>` | `tsp bench compare --skill-versions` (compare's flag, never `run`'s) only when `metadata.version` moved; `tsp bench gate --baseline bench/baseline --max-regression 0.05` applies the floors. `compare` and `gate` parse positionally, so a stray `--help` is read as a results path |
| plan template, ledger columns, refusal text | `go test ./internal/plan ./internal/assets`, then one `tsp plan init` render read by eye | Those tests pin the template bytes and the 14-column ledger header, so a green suite is the reading and the render is the check that it still reads as intended |
| `bench/` harness or corpus | `tsp bench compare` | It refuses across differing corpus digests or case sets: extending the corpus starts a new series, so re-baseline instead of comparing |
| a claimed gain in verdict quality | `bench-loop` | Train/held-out split, paired arms, three runs per arm. `measure-change` deliberately does not own that loop |
| shared code that also serves a command the request never named | `assets/paired.sh <base-ref> "<cmd>"` | Differential check: stdout, stderr and exit status compared against the base commit. Any difference nobody asked for is a regression to pin |

## Recipe: a code reading

```sh
make vet && make test                                    # the floor, before any claim
.claude/skills/measure-change/assets/paired.sh \
    -n 's/[0-9]+\.[0-9]+s/<s>/' \                        # instrument noise out, assertions in
    HEAD~1 'go test ./internal/plan -count=1'
tsp mutate --base HEAD~1 --fail-on-survivors 1           # survivors on the lines this change touched
```

Flags come before the base ref: everything after it is the command, so a trailing `-n` is handed to the
command and silently changes what ran. `paired.sh` puts the base side in a throwaway worktree, so
uncommitted work never leaks into "before", and it exits 0 identical, 1 different, 2 setup failure, 3
identical **because both sides failed the same way** — that last one compared two failures, not two
behaviours, and is no reading. `-n` takes a `sed -E` expression and may repeat. Normalize only what the
instrument says about itself — elapsed time, temp paths, ports — never the assertion text: a
normalizer that hides the payload turns a real difference into a green. Editing `paired.sh` owes
`assets/selftest.sh`, which asserts those four exit codes, that the base side really runs at the ref, and
that `-n` still hides instrument noise but not assertions.

## Recipe: a skill reading

```sh
git commit && make build                                  # a +dirty build cannot be re-derived
./bin/tsp bench run --cases '<set>' --runs 2 --runner pi --model "$MODEL" \
  --agent-config bench --max-cost-usd 15
./bin/tsp bench compare bench/results/<before> bench/results/<after>
./bin/tsp bench gate --baseline bench/baseline --max-regression 0.05 <run>
```

Two runs per side is the floor for a citable number: two runs of one unchanged configuration have
flipped four of seventeen cases, so a single arm is a reading and never a baseline. The throwaway agent
config must hold exactly the embedded `test-strategy` skill; a config carrying anything else fails the
run before it spends.

## Recording it

The reading goes in `docs/measurements/<topic>.md`, written from `assets/measurement-record.md`, or in
`bench/history.jsonl` when the bench produced it. Both are append-only evidence: a superseded reading
gets a new file naming its successor, never an edit that rewrites what was measured.

A reading whose "what this does not prove" section is empty is not finished. `docs/measurements/`
already carries three examples, including one that reports 3 of 7 agreement between the blind verifier
and the mechanical label and then names both disagreements as real defects.
