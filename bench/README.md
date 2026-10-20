# Benchmark corpus

Thirty small, realistic projects whose planted defects a "write tests that pass" approach never
finds. Every fixture ships with a README that states its contract honestly, source that violates
that contract in a specific collision the README does not mention, and a green happy-path suite a
reasonable developer would have written. The corpus exists to measure whether a testing flow
finds what the happy path hides, and to compare one version of the flow against another on the
same ground.

## Blind oracle verification (bench only)

After a finished `bench run --keep`, from the repository root:

```sh
tsp bench verify-oracle --run bench/results/<reading> --runner pi --model <pinned-model> --concurrency 2 --timeout 30m
```

One independent invocation per agent test file sees only that file and `contract.md`:
fixture README files, `docs/**/*.md`, the case request, and `go doc -all .` output for
Go packages. No implementation bodies, reference fixes, or answer-key defects are
copied into its temporary input directory. Test discovery matches the catch check.
`--dry-run` validates/discovers inputs without invoking an agent or writing oracle artifacts.
The case corpus must remain available under `bench/cases` (source cases use the case cache).

The strict JSON answer labels each named test `contract` or `invented`: a test may
only assert what the contract demands, so an undecidable test is invented, and the
judge cannot abstain. Malformed, truncated, incomplete, or failed answers become the
legacy label `unclear`, never rejection; readings judged before this prompt keep it.
Only `invented` witnesses are dropped. `<reading>/oracle/verdicts.json` records labels,
reasons, per-file invocation costs/turns, runner/model, fixed-prompt SHA-256 and scorer
revision; `summary.md` compares recorded and filtered caught defect-runs, red-on-fixed
tests and runs without witnesses. The original reading files are not rewritten. Oracle output symlinks (including the
output directory itself) are refused before invoking a judge; output files are replaced
without following existing file symlinks.
Unkept runs remain unchanged in both views. Catch records now name the tests red on the
fixed code. A reading built before they did is re-checked from its own kept workspace, and
only when the re-check reproduces the recorded counts; `--suite-timeout` caps that suite run.
A red test the re-check cannot name, or a re-check that disagrees with the reading, stays
unattributed and kept rather than guessing which test failed.

This judge is **uncalibrated**, not evidence of correctness. No real-model evaluation
or product integration is implied. Before using the labels as a gate, human-label a
held-out split, measure sensitivity/specificity with confidence intervals, and check
position, verbosity, format and same-family preference bias. Pin the exact verifier
model and compare paired valid readings in the same window. The preregistered T3 gate
is hard23 + real35 held-out, gpt-6-luna: at least halve tests red on fixed code while
losing at most 2 caught defect-runs per 100. Otherwise it stays a bench option.

## Independent review sessions

`tsp bench run --sessions N` measures a review/fix/review cycle. Each session starts a fresh
agent process with the plain prompt and case request, never a list of previous findings.
After each session, reference fixes remove the defects its tests distinguish; the agent's
tests remain. Source-referenced cases also apply the patch's recorded file deletions;
planted partial overlays never infer deletions from omitted files. Runs stop at N sessions or zero remaining defects. An unrepresentable remaining
set stops the run with a recorded reason (`fix/all` and `fix/keep-<ID>` are the supported variants).
A session that catches nothing leaves the workspace unchanged. Failed sessions retain their
work and fail the run after retries.

Per-session records include new catches, remaining defects, tests red on fixed code, turns,
cost and seconds. Summaries sum catches/remaining/broken tests over valid runs and count runs
reaching zero at each session. Provenance records the session limit; comparisons refuse mixed
limits, and history appends a sessions column. `--sessions` defaults to 1 (the original single
pass), must be positive, and above 1 cannot be combined with `--rounds` or `--escalate-model`.

## Reading plan-less runs and comparisons

For readings with no recorded plan, plan-only metrics (reported/found, recall, precision,
false positives, activation and plan adjudication counts) are unavailable, not zero.
New history rows and `tsp bench history` display `-`; existing Markdown rows and JSONL
records remain untouched. Run summaries and comparisons omit unavailable totals.
`caught` and `recall_caught` still measure the redcheck witness independently of a plan.
Mixed readings retain their legacy plan totals; they do not imply complete plan coverage.

`bench compare` prints whether the caught recall difference includes zero in a two-sided
95% Wilson/Newcombe interval (after minus before). Wilson score bounds use z=1.959963984540054;
the difference bounds combine the relevant arm errors in quadrature (Newcombe's unpaired
method, without continuity correction). For example, 16/18 versus 15/18 is within noise.
This descriptive test treats defect-runs as independent; repetitions and defects in the
same case may be correlated, so this is not proof of equivalence or a skill-version win.
No defect-run denominator produces an explicit unavailable verdict.

## Layout

```
bench/cases/<id>/
  KEY.json          the answer key: defects, exact lines, triggers with observed output
  fixture/          the project the agent sees: README.md, src (or *.go), tests, package.json / go.mod
  fix/all/          corrected copies of only the defective files, at the same relative paths
  fix/keep-<ID>/    every other defect fixed, defect <ID> left exactly as in fixture/ (multi-defect cases)
```

A clean negative control ships `KEY.json` and `fixture/` only: with nothing planted there is nothing
to fix and no variant to overlay.

### Real-bug cases (r*), shipped by reference

```
bench/cases/<id>/
  KEY.json          the answer key, as above; `source` names the go-swe-bench-v0 record
  SOURCE.json       {"repo": "https://github.com/<owner>/<repo>", "commit": "<parent sha>", "patch": "fix.patch"}
  fix.patch         the upstream fix, applied with -p1
```

A real-bug case is a whole upstream project at the commit before its fix, so it is not copied into this
repository (35 cases would add 247 MB of third-party code). `bench run`, `bench score` and `bench rescore`
build it on first use into the user cache directory (`~/.cache/tsp/bench-cases` on Linux): `fixture/` is
the commit's tree without history,
`fix/all/` is that tree with `fix.patch` applied. Only that commit is fetched, later runs reuse the cache,
and a case that cannot be built (no network, a patch that no longer applies) stops the run rather than
silently shrinking the corpus. Real commits fix one defect each, so these cases ship no `fix/keep-*`.

The built tree is the upstream tree exactly. 25 of the 35 cases match the copies earlier readings were
taken on byte for byte; the other 10 had lost files while vendored (paths the upstream's own `.gitignore`
or this repository's `bin/` rule excludes, empty submodule directories, symlinks copied as files, and a
hand-edited `go.mod` without `go.sum` in r30). Readings of those 10 before the move were taken on the
filtered trees.

Node fixtures run with `node --test` (ESM, no dependencies). Go fixtures run with `go test ./...`
(standard library only). Every suite is green with the defects present.

## Fixed versions

`fix/all/` holds the corrected version of every file named in `KEY.json`, and nothing else. For a
case with more than one defect, `fix/keep-<ID>/` fixes every defect except `<ID>`, which stays
byte-for-byte as in `fixture/`; a single-defect case has only `fix/all/`. Overlaying a variant on
a copy of `fixture/` gives a project whose happy-path suite is still green.

The agent never sees `fix/`. The runner uses it to attribute a catch from suite exit codes alone:
the agent's tests must be green on `fixture/` + `fix/all/`, and defect D counts as caught when the
same tests are red on `fixture/` + `fix/keep-D/`. A test that is green on every variant never
touched a defect, whatever its finding text says.

Known limits of the overlays, verified by running every trigger against every variant:

- `n05-keyset-pagination`: the happy suite pins the cursor to a plain `created_at` string, so the
  fix appends `#<id>` to the cursor only when another row shares that `created_at`.
- `n09-json-ids`: JavaScript numbers cannot hold the keyed id, so the fix parses integer ids as
  `BigInt`; `trigger.expected` lists the values, not their runtime type.
- `n04-slug-normalize`: the fix NFC-normalizes the title and keeps non-ASCII letters as
  separators (`café` and `café` both become `caf`), the second form the key accepts; folding
  accents would add behaviour the contract does not promise.
- `n03-subscription-window`: the fix reads the calendar day from the UTC clock, so a
  `new Date("YYYY-MM-DD")` stays on that day in every zone; a local-midnight `Date` in a zone east
  of UTC still lands on the previous day, a limit of representing a day as an instant.
- `n08-sliding-limiter`: the D2 trigger as written (limit 1, one prior hit) is also red while D1
  is present, so a test copied from it is attributed to both defects. A probe that isolates D2
  makes two prior hits on `token:ABC` before calling `token:abc`.

## Key schema

```json
{"id":"<id>","language":"node|go","suite":"node --test|go test ./...","surface":"library|cli|http|worker|etl|state-machine",
 "defects":[{"id":"D1","file":"src/x.js","line":42,"class":"<class>","keywords":["k1","k2","k3"],
   "description":"one sentence, what is wrong",
   "trigger":{"input":"<exact call or command>","expected":"<per contract>","actual":"<observed>"},
   "why_missed":"one sentence: why a happy-path suite and branch coverage never reach it"}],
 "traps":[{"id":"t1","file":"src/x.js","line":57,"class":"<class>","keywords":["k1","k2"],
   "description":"one sentence, the tempting behavior",
   "documented":"README.md:<line of the sentence that states it is intentional>"}]}
```

`line` is the line of the defective statement in `file` (relative to `fixture/`). `keywords` are
three to five lowercase words a correct finding would contain. `trigger.actual` is pasted from a
real execution against the fixture; a defect whose trigger was never run does not belong in a key.

`traps` is optional: documented behaviors planted next to the real defects. A report of a trap is
an adjudicated false positive, never a catch: traps are not defects, so they are excluded from
defect totals, the `fix/` overlay requirement, mechanical matching, and caught attribution by
construction, and adjudication refuses to confirm a trap id as a defect. Trap keywords must be
 disjoint from the case's defect keywords, and a trap's line must sit more than five lines from
every defect line, so a trap report can never be mechanically mis-attributed to a real defect.
Each entry's `documented` names the fixture README line that states the behavior is intentional.

## Cost and wall clock

The cost of a run is the number of agent turns; concurrency does not change it. The wall clock is
another matter: cases are independent, so `--concurrency N` runs N of them side by side and the
run takes about as long as its slowest case instead of the sum of all of them. In the four runs
measured so far the sum was 26 to 39 minutes and the slowest single case was 5.4 minutes.

`--agent-config bench` builds a throwaway Claude configuration holding only the skill under test
(the embedded `test-strategy`; the other embedded skills would ride every turn as catalog entries),
with the operator's credentials symlinked in. A case then measures the skills rather than whatever
else the machine makes a session do: in one 15-case run, 82 tool calls went to the memory protocol
and to loading it, roughly a fifth of the turns.

Trimming the corpus is the lever that does cost quality. Of the fifteen cases in that measurement, thirteen produced
different numbers across the four skill versions measured so far; only `n01` and `n05` were
identical every time. There is little to remove.

## Scoring

The runner copies `fixture/` into a fresh workspace, runs the flow under evaluation there, and
reads the plan the workspace declares in `.tpp.json`, or `docs/testing/test-plan.md` when it
declares none. `result.json` records the path it read as `plan_path`. A declaration that cannot be
read or that escapes the workspace is refused: the default path is read instead, and the refusal is
named in the run's notes so a run is never silently scored as having delivered no plan. For every
key defect:

- **found** when a finding row, together with the Evidence ledger rows it cites, names the same
  file and either a line within ±5 of the key line or any of the key's keywords;
- **missed** otherwise.

A finding row must cite at least one id that exists in the Evidence ledger; a row that names the
defect without linked evidence is recorded as `unlinked` and does not count, whatever it says. An
empty ledger makes every citation dangling.

A plan whose findings are prose scores zero exactly like a plan with no findings, so the run
records which of the two happened: `plan_format` is `table`, `prose` or `empty`, and a prose plan
carries a note saying nothing in it can be located or re-scored.

One finding row is one claim: a row is credited to the defect it names by keyword (to several
only if it names several), and a row that matches by line alone goes to the nearest defect. Four
cases plant two defects within the line tolerance of each other, so without this a run that
noticed one of them would read as having noticed both.

A **false positive** is a finding row a decision says is wrong. Lexical and location matching only
**proposes** an association; nothing is a false positive by default. A row that matches nothing is
**unmatched**, and a row nobody has decided is **pending**, which is what the summary, the
aggregate and the history report.

Recall is found over key defects; precision is found over findings.

### The three facts about one defect

- **reported** is the mechanical measure above: a row that cites the defect's file and either a
  line within ±5 or one of its keywords counts. It needs no reviewer and is comparable across runs.
- **confirmed** is the adjudicated measure: the row counts only once a person, or a recorded rule,
  has said so through the record below.
- **caught** is the test-verified measure: the agent's own tests distinguish the defective code from
  the fixed one, plan or no plan.

They are reported side by side, and a run that reports without confirming, or confirms without
catching, shows up as a gap between the columns rather than as a single score.

### The adjudication record

`bench adjudicate` writes `<run>/adjudication.json` beside the plan and `result.json` of one run:

```
tpp bench adjudicate --run bench/results/<ts>/<case>/<run> --pending
tpp bench adjudicate --run <run dir> --row 2 --verdict false_positive \
  --by alesierraalta --reason "style claim, not a defect of this candidate"
tpp bench adjudicate --run <run dir> --row 2 --verdict defect --defect D1 \
  --by alesierraalta --reason "the row names the defect the key plants" --replace
```

One decision per finding row, named by its 1-based index in the Findings table. Every decision
carries the row's fingerprint, so one taken against different text is refused instead of applied,
and `--replace` keeps the displaced decision under `superseded` instead of erasing the audit
trail. The verdicts are:

- `defect` — the row genuinely reports a keyed defect, which may be one matching never proposed;
- `false_positive` — the row claims a defect that is not there;
- `out_of_scope` — a valid observation outside the key (style, documentation, a pre-existing issue
  the corpus does not plant). It is never a false positive and stays out of the precision
  denominator.

`bench rescore` reads the record of a run from that run's own directory, which the runner owns.
`bench score` applies a record only when `--adjudication <path>` names it: it is never discovered
beside the plan, because the plan a workspace holds sits in the area the evaluated subject writes,
and a record found there would let the subject rule on its own finding rows. Naming a record that is
not there is a refusal, not a silent skip.

**Precision** is `confirmed / (confirmed + false positives)` over finding rows. It is undefined
until at least one row has a decision — `null` in the JSON, `none` in the summary — and it is
printed next to its pending count, so a run with zero recorded false positives is never read as a
perfect one. `bench compare` refuses to compare two readings recorded under different metrics
versions: version 1 counted an unmatched finding as a false positive, version 2 does not.

### Clean negative controls

Two cases plant nothing: `c01-clean-allocate` and `g05-clean-backoff`. A key declares
`"control": "clean"` and no defects; a key with no defects that does not say so is still refused,
and a control that carries a defect is refused too. A control contributes no defect to any
denominator — its recall cells read `clean`, not `0/0` — and it measures the other side of the
ledger: every finding row reported there is unmatched until decided, so a flow that invents
findings shows up as rows to adjudicate and, once decided, as false positives.

That is the **reported** measure. The **caught** measure asks whether the agent's tests distinguish
the defective code from the correct one, plan or no plan: the test files the agent added or changed
are carried onto `fixture/` + `fix/all/` (they must be green there) and onto `fixture/` +
`fix/keep-<ID>/` (red means some test distinguishes `<ID>`). Both measures are recorded per defect,
in the summary, the aggregate, and the history. A run that reports a defect without a test that
catches it, or catches it without reporting it, shows up as a gap between the two columns.

A defect whose class names nondeterminism (`race`, `concurrency`, `timing`, `flaky`) has its
variant run three times instead of once: one red attempt proves the test distinguishes it, while
a green run proves nothing. Without that, g01's race scored 1 of 2 or 2 of 2 from the same
workspace depending on the interleaving, even under `-race`.

A test red on `fix/all` but green on `fix/keep-D` asserts the defective behaviour: it locks the
bug in and goes red the moment someone fixes it. Those are counted as **inverted** and named in
the run's notes, apart from tests that are simply red everywhere.

**Pinned** is the third column: it counts defects whose finding names a test in the plan's
`Pinning test` cell. It measures the claim, caught measures the outcome, and a run that pins more
than it catches is naming tests that distinguish nothing.

Every run keeps the plan it produced as `test-plan.md` beside its `result.json`, whatever path the
workspace declared, even when the workspace is removed, so older runs can be re-scored when the
rule changes: `tpp bench score --case bench/cases/<id> --plan <results>/<id>/<run>/test-plan.md`.

A valid run whose selected plan file is absent scores zero and is reported as `NO PLAN` (`no_plan` in the
aggregate and the history): the flow ran and did not persist its deliverable at the path it promised,
which is a different failure from missing the defect. The selected path is the one the workspace
declares, else `docs/testing/test-plan.md`; the plan read must resolve inside the workspace, and a path that
resolves outside through a symlink is refused like an absolute or `..` declaration. A workspace or a plan path
the bench cannot resolve is refused as well, because containment that cannot be checked is not containment;
a plan path that is simply not there stays the ordinary `NO PLAN`. A rejected declaration does not become a way
around the guard: when it falls back to the default path, that path is checked like any other.
A run that declares a
path, does not write it and leaves a plan at the default path also reads `NO PLAN` — the declaration is the
run's promise. A declared-but-absent
plan reports the declaration it honoured alongside the default plan it ignored. When the selected path is
there but cannot be read as a plan, both scorers name the read failure in the result and score zero, while the
adjudicated scorer additionally returns it as an error, making `rescore` fail loudly rather than silently
scoring an unreadable kept copy. Runs where the agent did not complete are `FAILED`, excluded from recall, and
make the command exit 3.

### Skill versions in the history

The history's `skill version` column records the `version` field of the installed `test-strategy`
skill. Releases up to 3.4 were labelled `3.N`; the same lineage is written `0.3.N` from 2026-09-10
onward (`3.0`…`3.4` ≡ `0.3.0`…`0.3.4`). Rows recorded under the old labels stay as they were
written: the history is append-only, so a rename would rewrite evidence instead of adding to it.

### Denominators and variability

Every number is reported in a named unit, and the summary, the aggregate and the comparison name
it:

- **defect-runs**: keyed defects over valid runs. Two runs of one defect are two entries; this is
  the unit behind `Recall` and `RecallCaught`.
- **unique defects**: distinct `(case, defect id)` pairs, counted once however many of its runs
  found, confirmed or caught it. This is the unit behind `recall_unique`, `recall_unique_caught`,
  `unique_defects`, `unique_found`, `unique_confirmed` and `unique_caught`.

A case whose valid runs disagree with each other is named as unstable in the summary and in the
history row, so variability is a reading rather than a footnote. A keyed defect the catch check
could not reach at all — its variant never ran — is counted as **inconclusive**, which is not the
same answer as a defect no test distinguished.

### The baseline

A number is citable only together with its instrument: the corpus commit, the scorer build, the
skill version, the model, the runner, the agent-config mode, the environment, and at least two runs
per configuration. One run of a `+dirty` build is a
reading, not a baseline, and `bench compare` refuses two runs whose case sets differ, whose recorded
corpus digests differ, or whose per-case defect counts differ, so extending or editing the corpus
starts a new series instead of a delta. It refuses them for the rest of the instrument too: a
different model, runner, skill version, number of runs per case, agent-config mode, environment
(os/arch) or metrics version is a field that moved, and the refusal names it. A skill version change is the one
field an operator can accept on purpose: `bench compare --skill-versions` compares two skill versions on
an otherwise identical instrument and says so in its header. A difference in the
suite runtimes (`node`, `go`) is reported in the comparison header without a refusal, because a
refusal there would block every pair across a toolchain patch. The throwaway agent config is
verified to hold exactly the embedded `test-strategy` skill: a config that quietly carries anything else fails the
run before it spends, which is what keeps an inherited configuration or a stray memory skill out of
the measurement. Every run records `corpus`, a digest over the case names and
defect ids it measured, in its aggregate and in the history's `corpus` column; a
case that failed or was invalid still contributes its name and key defect ids to that digest.

Each valid run also records which plan mode it wrote: `light` counts plans that declare a validated
scoped run, and `micro` counts activated micro plans (declared and accepted by `plan check`). The two
are counted independently — plan check refuses a plan that is both — per case in the summary, in the
aggregate, and in the history's `micro` column. Rows written before that
column sit under a note saying they record no micro plan either way. `bench compare` prints both counts
before → after beside the turn and cost totals, so a Micro A/B reads its cost next to its activation.

```
git commit                      # a build from a dirty tree prints a warning and cannot be re-derived
make build
bin/tpp bench run --cases '*' --runs 2 --model sonnet --agent-config bench --max-cost-usd 40
```

Eighteen cases at two runs each is roughly 36 agent runs; the last fifteen-case run cost $12.83, so
the ceiling above stops the run just past where it would have spent what the corpus is worth.

The history's `rounds` column is appended last and always records passes per run (1 for a one-pass
reading). Rows without the key predate the column; they did not record a round count.

Use `--rounds N` to run all N rounds in the same workspace, each in a fresh agent process. Later
rounds receive the tests earlier rounds left failing on the unmodified code, and keep those tests.
Each round records witnesses, new witnesses, caught defects, tests red on the fully fixed code,
turns, cost and seconds; the summary totals catches and broken tests by round over valid runs.
The stop-on-no-new-witness rule can be evaluated offline. A later round that exhausts retries fails
the run. Counts above 1 cannot be combined with `--escalate-model`, and `bench compare` refuses
mixed round counts. Omitting the flag or using 1 preserves the one-pass behavior.

## What a reading can decide

One run per case decides nothing. Two runs of the same configuration have flipped four of seventeen
cases and moved the turn total by 13%; at three runs a case, five of the eighteen cases still change
their own outcome (`n01`, `n02`, `n06`, `n08`, `n12`). Treat a per-case verdict as a sample, and a claim
about the corpus as a claim about a distribution: the totals of one pass are not a result.

The digest identifies the measurement — the cases, the request each one is asked for, and the runs per
case — so a reading that changed any of those is not comparable with one that did not, and `bench
compare` refuses when the two digests differ.

## Rules

- The key never travels into the workspace the agent sees. Only `fixture/` is copied.
- Fixtures are never edited to make a run pass. A wrong key is fixed in the key; a weak defect is
  replaced by a real one and re-verified.
- Suites stay green with the defects present. A suite that goes red has stopped being the happy
  path and no longer measures anything.

## Adding a case

1. Pick a collision the contract is silent about (two features meeting, a boundary, a second
   call, a representation limit, a missing lock).
2. Write the README as the project would; state the guarantee, not the trap.
3. Implement realistically, without comments that point at the defect.
4. Write the suite a busy developer would write; run it; it must pass.
5. Execute every trigger; paste the observed output verbatim into the key.
6. Record the exact line of each defective statement.

## Cases

| id | language | surface | defects | classes |
|---|---|---|---|---|
| n01-csv-rfc4180 | node | library | 2 | escaping, parsing |
| n02-money-rounding | node | library | 2 | float-precision, rounding |
| n03-subscription-window | node | library | 2 | timezone, calendar |
| n04-slug-normalize | node | library | 2 | idempotence, unicode-normalization |
| n05-keyset-pagination | node | library | 1 | pagination-cursor |
| n06-retry-policy | node | http | 2 | unbounded-retry, fail-open |
| n07-tenant-scope | node | library | 2 | authorization |
| n08-sliding-limiter | node | library | 2 | off-by-one, normalization |
| n09-json-ids | node | etl | 1 | integer-precision |
| n10-order-state | node | state-machine | 2 | unreachable-state, state-machine |
| n11-html-escape | node | library | 2 | injection, idempotence |
| n12-path-normalize | node | http | 1 | path-traversal |
| n13-vacuous-assert | node | library | 1 | boundary-clamp |
| n14-protected-bug | node | library | 2 | rounding, off-by-one |
| g01-inventory-reserve | go | library | 2 | data-race, off-by-one |
| g02-config-merge | go | library | 2 | config-merge |
| g03-batch-writer | go | worker | 2 | data-loss, error-reporting |
| g04-skipped-guard | go | library | 1 | inclusive-boundary |
| n15-contract-compat | node | http | 2 | signature-reserialized-body, n-1-payload-incompatible |
| n16-crash-state | node | cli | 2 | non-atomic-write, swallowed-write-error |
| n17-event-consumer | node | worker | 2 | non-idempotent-consumer, out-of-order-overwrite |
| n18-dependency-client | node | http | 2 | deadline-not-propagated, retry-without-idempotency-key |
| n19-reconnect-listener-leak | node | library | 1 | listener-leak |
| n20-batched-query-amplification | node | library | 1 | query-amplification |
| n21-retained-cache-growth | node | library | 1 | memory-retention |
| n22-hot-path-scaling | node | library | 1 | hot-path-scaling |
| n23-independent-io-serialization | node | http | 1 | io-serialization |
| n24-go-allocation-benchmark | go | library | 1 | allocation-churn |
| c01-clean-allocate | node | library | 0 (clean control) | — |
| g05-clean-backoff | go | library | 0 (clean control) | — |

Forty-five defects across twenty-eight defective cases, plus two clean controls that plant nothing.

### The guarded-defect family

`n01` through `g03` all plant defects an unguarded happy path never reaches: the collision sits
where no test looks. `n13-vacuous-assert`, `n14-protected-bug`, and `g04-skipped-guard` plant a
different failure: a test does look, and is green anyway, for a reason that has nothing to do with
correctness. `n13` names the exact behaviour in a test that asserts a property true of every
return value. `n14` goes further: its first defect's test asserts the *defective* output as the
expected value, so the green suite is not merely blind but actively pins the bug — a correct fix
turns that test red, which is why `fix/all` and `fix/keep-D2` (D1's function is corrected in both)
also carry a corrected assertion, never just the source. `g04` gates its boundary test behind an
environment variable this fixture never sets, so `go test` reports `ok` while the guard never runs.
Finding these needs no new scoring machinery — the defect still lives in production code the
scorer already locates by file and line — but it does mean an agent has to read what a green test
actually proves instead of trusting that it was written to prove something.
