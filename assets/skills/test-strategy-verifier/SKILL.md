---
name: test-strategy-verifier
description: "Trigger: terminé la implementación, implementation done, a code change is complete, ready for review, before RDD, ¿quieres hacer la validación del TSP?, validación del TSP, run the TSP validation, verificar el cambio con TSP, verificar este cambio, TSP validation gate, post-implementation verification. Own TSP verifier: after ODD finishes an implementation and before the RDD review, it asks the repository whether THIS work changed production source and owes a test, then runs the full test-strategy flow without asking the human. It never fires after a report, a document or a note, which implement nothing."
license: Apache-2.0
metadata:
  author: "alesierraalta"
  version: "0.2.3"
  requires_tsp: "0.7.0"
  scope: [common]
  auto_invoke: "After ODD finishes an implementation and before the RDD review or declaring done: run `tsp check` with one `--paths` per changed production source path (e.g. `tsp check --paths a.go --paths b.go`), say the verdict in one line, then run the full test-strategy flow when it is owed. No production source from this work means no implementation, so no TSP."
---

# TSP Own Verifier

## Activation Contract

This is TSP's own verifier. It takes its place in one order and only there: **ODD implements, TSP
verifies the implementation, RDD reviews it with hashes.** So it runs after an implementation is
finished and before the RDD review when RDD is present, and immediately after the implementation
when there is no RDD. It does not hand the change to Gentle AI's verifier agents; TSP validates its
own change with its own flow.

An implementation is production source this work changed. A report, a document, a note, a ledger row
or a chat answer implements nothing: those runs finish without TSP, and no gate is opened for them
however much source other work left behind in the tree.

The moment an implementation lands (code or behavior changed by this work, work unit complete,
ready for review), run the Verification Gate below BEFORE declaring the work done, before the RDD
review, and before opening a PR or delivering.

## Verification Gate — the repository answers, not the human

Never ask the human whether to validate, nor what or where to validate. Both answers already live
in the repository, and a question the agent could answer itself turns a criterion into chat noise.

Run `tsp check` with one `--paths` per changed production source path (e.g.
`tsp check --paths a.go --paths b.go`) before declaring the work done.
The paths come from the work itself — the files this work unit edited — never from the whole
repository, which carries debt older than this work.

- **exit 0**, or a report that no production source is changed in those paths → TSP is not owed.
  Say it in one line (`tsp check: nothing to validate`) and continue: no flow, no question.
- **exit 1** → the report names what this change is missing, in the check's own wording: it needs a
  test beside it, or it could not be judged. That report is the scope; run the flow over it.
- **a plan target still open** is progress through a run's agenda, never this change's debt: the
  scoped report never asks for one, and neither does this gate. Only an unscoped reading sees them,
  and that reading is not this gate.
- **exit 1 that prints nothing**, or **any other non-zero exit** (a usage error is the check
  failing, not a debt) → the check could not answer here. Apply the same criterion by hand:
  an uncommitted change to production source in this work unit is owed; documentation, fixtures
  and ledger-only edits are not — which is also why a report-only run never reaches this line. The Pi host's TSP offer stays silent in exactly these
  could-not-answer cases, so that hand rule is the agent's own fallback and the two never
  contradict.

An explicit human instruction to skip TSP for this change is honored for that change alone: record
it in one line and proceed. That is a decision the human already made, not a question to ask.

## Scope — derived, never asked

Validate what the gate named, not the repository: the changed paths in the report, the breadth
`tsp plan gaps` names when a plan exists, and the blast radius of the changed symbols. When the
report names several owed items, cover all of them in one flow rather than one flow per item.

## Where to run it — derived, never asked

Choose the target from the change and the suite, and say it in the same line as the verdict:

- **this machine** by default: no transport, fastest feedback.
- **the Raspberry Pi** only when parity is the point — the change carries an architecture-specific
  path, build tag, syscall or cross-compiled artifact that has to be exercised on aarch64.
- **the laptop** only for capacity — the suite cannot finish locally in a bounded run. Measure it
  locally first, and name the number that justified moving.

If the derived target is unreachable, say so in one line and run the same flow on this machine.
Never fall back to asking which one to use.

## On owed — the full test-strategy flow

Delegate the validation to the test-strategy skill and run it to completion over this change. Do
not truncate it to a quick check and do not stop at a green suite. The flow is the persisted
plan: inventory the change, sweep layers, rank targets, and execute each target through sandboxed
probes with executed evidence — no finding from reading alone. The verification reports what was
validated and what was NOT reachable, and every finding carries an executed evidence record per
`~/.claude/skills/test-strategy/references/evidence.md`.

Treat this as verification, not as a new implementation: fix only what the flow proves broken,
and re-run the affected targets until they are green on the final tree.

## Read Boundaries

- This gate decides whether to validate and over what, from repository state; it does not review
  the code and does not replace the RDD review, native review authority, receipts, consent
  envelopes, or delivery policy.
- The gate reads the repository and runs one flow. It never asks the human, never invents a second
  gate, never widens the scope past what the report names, and never records a skipped validation
  as a success.
- Read-only outside the test-strategy flow this skill delegates to; the flow owns plan and
  evidence writes.
