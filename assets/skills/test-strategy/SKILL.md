---
name: test-strategy
description: "Trigger: haz el TSP, haz TSP, TSP, haz test stragety, haz test strategy, test-strategy, haz el testing, testea esto, prueba esto, test this, test the app, test my change, run the testing, calibra el testing, calibrate the testing, test strategy, what to test, where to start testing, test planning, testing priorities, test altitude, low coverage, legacy code testing, test plan for the app, testing roadmap, plan de testing, roadmap de pruebas. Find the defects a green suite hides and pin each one with a failing test."
license: Apache-2.0
metadata:
  author: "alesierraalta"
  version: "0.7.8"
  requires_tsp: "0.7.1"
  requires_tpp: "0.5.1"
  scope: [common]
  auto_invoke: "Any request to test something: find the defects the green suite misses and pin each one with a failing test"
---

Goal: find the defects the existing green suite misses, and leave one test per defect in the
suite that is RED on today's code and would be GREEN once the defect is fixed. What a run leaves
behind is the suite it committed, not a regression net: a witness that stays local (a `testLocales/` directory, an
untracked file, a probe that only exists as a command someone ran) is not in the repository's suite,
so the run names the ones that stayed local instead of implying a wider net than it cast. Do not
change source code. Spend turns on probes and tests, not on documents.

Work promise by promise. A pass over the code is not the unit of work: coverage of
promise × class is, and step 6 says when it is complete.

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

1. **Harvest the promises, from four sources, not one.** A promise is anything the code says it
   will do; every sentence and every name that makes a claim is a probe waiting to happen.
   - The contract: README, specs, doc comments, the documented examples and their stated limits.
   - Every line that asserts something: names that claim a behaviour (`sorted`, `unique`,
     `cached`, `safe`, `idempotent`, `atomic`, `bounded`), comments, type aliases, validation
     branches, and the branches the code does not take. A name that lies is a defect, so treat
     each name as a promise and probe it.
   - The failure classes in step 2: each is a promise the code makes implicitly ("never leaks",
     "never panics", "at most one", "never empty").
   - The evidence this session already has: the diff, the bug report, the log line that started
     the work. A change is a promise list: everything the changed code touches inherits its
     promises, including the callers nobody named.
   Keep the list in your head as `path:line — promise`; never write a document.

2. **Attack every promise by every class it can break in**, not just the documented examples:
   empty and zero, boundaries (exact limit, one past it), non-ASCII and Unicode equivalence
   (composed vs decomposed, case folding like ß/İ), quoting and embedded separators or newlines,
   malformed input, large magnitude or many repetitions (float drift, growth), repeated
   application (idempotence, round trip, parts summing to the whole), time zones and DST,
   concurrency, and every error, rejection and exit-status path. Batch many probes into one
   command (`node --input-type=module -e '...'`, a scratch `go test`) so one turn checks many
   inputs. Prefix every command with `timeout 20` and give tests a timeout
   (`node --test --test-timeout=5000`, `go test -timeout 30s`): a defect that hangs, even in a
   synchronous loop, must fail fast.
   **Resources must not grow.** Whatever the code holds — memory, open files or sockets,
   goroutines or threads, temp files, cache or index entries, connection slots, subscriptions —
   must not grow with repetition or with failures. Drive the same operation N times and then 2N
   and compare a measured quantity (RSS, `/proc/self/fd`, live goroutines, files left behind,
   entries kept): the marginal growth must be zero, and a leak shows as a number that keeps
   climbing. Measure at a size where a leak is visible (thousands, not three), keep the results
   reachable so nothing is collected before you measure, and drive the path that fails too: a
   leak on the error path counts the same as one on the happy path.
   **Load must not degrade the answer.** For a promise about throughput, latency, capacity or
   isolation, run the same correct workload at 1× and at 10× concurrency and check the promise
   still holds: same answer, no error, no growth, latency inside the bound the contract states.
   If the contract states no bound, the probe is "no wrong answer and no growth" — never a
   number you invented.
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
   **A probe you cannot falsify proves nothing.** Before you accept that a promise holds, name the
   input or the state that would have made the probe fail. If you cannot name it, the probe is
   blind: it will report success on broken code. A probe that passes both a value that satisfies
   the promise and one that violates it measured nothing, and so did a counter that reads the same
   at every size.

3. **The oracle is the contract, never today's output.** If a probe's result differs from what the
   contract promises, that is a defect. If you only know what the code does now, it is not a test.
   Existing tests show today's behaviour too: never copy their expected messages or values as the
   oracle for a promise the contract states differently or not at all.

4. **Pin every defect** in the repository's own test files: one test per defect, asserting the
   promised behaviour, so it fails now. Assert only what the contract states: when it does not name
   a field, type or message, check the weakest observable that proves the promise (it rejects; the
   error mentions the item), never a name you invented. A test pins the whole rule, so it asserts
   every effect the rule promises, and for a rule about a rejection it also asserts that the
   rejected input changed nothing it was supposed to leave alone (stored state, counters, files).
   The test must pass on any correct fix. Run the suite once at the end to see those tests red and
   the rest green; never delete or weaken a test to make the suite green.

5. **Sweep, and say what is left.** A pinned defect is one cell of the matrix, not the end of the
   pass: after each pin, go back over the promises you listed and the classes you have not yet
   driven and probe those. Each sweep starts from what is still unprobed, never from the start.

6. **Stop when the matrix is covered, not when the suite is green.** The sweep is complete when
   every promise is either pinned by a test for every class that applies to it, or explicitly ruled
   out with the reason it cannot happen here (the surface is absent, the contract forbids the
   input, the operation has no state). A green suite is not evidence of coverage: it only says the
   tests you wrote pass. When a class is left unprobed, say so instead of calling the work done.

7. **Finish** with a short list: each defect as `path:line` plus the test that pins it, the cells
   that stayed unprobed with their reason, and which witnesses stayed local instead of landing in
   the suite.
