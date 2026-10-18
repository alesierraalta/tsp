---
name: test-strategy
description: "Trigger: haz el TSP, haz TSP, TSP, haz test stragety, haz test strategy, test-strategy, haz el testing, testea esto, prueba esto, test this, test the app, test my change, run the testing, calibra el testing, calibrate the testing, test strategy, what to test, where to start testing, test planning, testing priorities, test altitude, low coverage, legacy code testing, test plan for the app, testing roadmap, plan de testing, roadmap de pruebas. Find the defects a green suite hides and pin each one with a failing test."
license: Apache-2.0
metadata:
  author: "alesierraalta"
  version: "0.7.1"
  requires_tsp: "0.6.6"
  requires_tpp: "0.5.1"
  scope: [common]
  auto_invoke: "Any request to test something: find the defects the green suite misses and pin each one with a failing test"
---

Goal: find the defects the existing green suite misses, and leave one test per defect in the
suite that is RED on today's code and would be GREEN once the defect is fixed. Do not change
source code. Spend turns on probes and tests, not on documents.

**Route special surfaces.** Look at what the code under test is made of (manifests, imports, directories). For each surface below that it actually has, read that skill's `SKILL.md` and apply its probes to that surface in steps 2-4; its defects are pinned the same way, one failing test each. Skip a row whose surface is absent and a skill that is not installed; plain logic needs none of them.
- RAG retrieval, vector search, embeddings, citations: `rag-audit-evaluator`
- LLM features, prompts, LLM judges: `llm-eval-design`; untrusted content reaching an LLM that holds tools: `llm-redteam`
- Agents that call tools and change state: `agent-eval`
- Terraform or other infrastructure as code: `iac-safe-auditor`
- Database schema, migrations, transactions: `database-persistence-testing`
- Queue, event or webhook consumers: `messaging-eventdriven-testing`
- An API, schema or payload that others consume: `contract-compat-testing`
- Files or durable state that must survive a kill or a full disk, CLIs, signal handlers: `crash-and-process-testing`
- Calls to remote dependencies (timeouts, retries, circuit breakers): `resilience-fault-injection`
- Authentication, permissions, parsing untrusted input, secrets: `appsec-adversarial-auditor`

1. **Read the contract once.** README, doc comments, the source files, the existing tests. Write
   down (in your head, not a file) every promise the contract makes.
2. **Probe each promise across its input classes**, not just the documented examples: empty and
   zero, boundaries (exact limit, one past it), non-ASCII and Unicode equivalence (composed vs
   decomposed, case folding like ß/İ), quoting and embedded separators or newlines, malformed
   input, large magnitude or many repetitions (float drift, growth), repeated application
   (idempotence, round trip, parts summing to the whole), time zones and DST, concurrency, and
   every error, rejection and exit-status path. Batch many probes into one command
   (`node --input-type=module -e '...'`, a scratch `go test`) so one turn checks many inputs.
   Prefix every command with `timeout 20` and give tests a timeout (`node --test --test-timeout=5000`,
   `go test -timeout 30s`): a defect that hangs, even in a synchronous loop, must fail fast.
   When a promise holds for a whole family of inputs (relative order, round trip, never panics,
   parts summing to the whole, idempotence), do not pick the examples yourself: write it as a
   property test that generates many inputs (sizes, positions, values) with a seeded random loop or
   the language's fuzzer (`testing/quick`, `go test -fuzz`, `fast-check`) and asserts the property
   for each. When the input must pass a validator first (checksum, signature, schema), build it
   with the code's own encoder or constructor and then perturb it. Pin the first counterexample as
   its own example test.
   Code under test usually hides more than one defect: after you pin one, keep probing every
   remaining promise and every public operation (each exported function, each command path).
   An invariant ("never", "always", "at most") must hold after every operation that changes
   state, including refunds, undo, reset and retries: drive each one and check it afterwards.
   For equality, dedupe and lookup promises assert the relation (equivalent inputs give equal
   results, `f(a) === f(b)`), not one fixed output value.
   For a promise about cost, work or retention (I/O, allocations, comparisons, retries, entries
   kept) the probe must count the work directly at a size where the defect can show (hundreds to
   thousands of items, or a value big enough to accumulate error), never infer it from one correct
   output. Then check that the counter moves: run it once at one item and once at thousands — if it
   reads the same (or zero) at both, the probe is not measuring the work: keep the measured value
   in a sink or an accumulated total so it cannot be optimized away. For a sequence promise, drive
   the whole sequence the contract describes — the defect often shows only on the third attempt,
   the second rollover or a late operation, not the first.
3. **The oracle is the contract, never today's output.** If a probe's result differs from what the
   contract promises, that is a defect. If you only know what the code does now, it is not a test.
4. **Pin every defect** in the repository's own test files: one test per defect, asserting the
   promised behaviour, so it fails now. Assert only what the contract states: when it does not name
   a field, type or message, check the weakest observable that proves the promise (it rejects; the
   error mentions the item), never a name you invented. The test must pass on any correct fix. Run the suite once at the end to see those tests red and
   the rest green; never delete or weaken a test to make the suite green.
5. **Finish** with a short list: each defect as `path:line` plus the test that pins it.
