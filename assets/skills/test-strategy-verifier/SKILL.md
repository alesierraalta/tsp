---
name: test-strategy-verifier
description: "Trigger: terminé la implementación, implementation done, ready for review, before RDD, ¿quieres hacer la validación del TSP?, validación del TSP, run the TSP validation, verificar el cambio con TSP, verificar este cambio, TSP validation gate, post-implementation verification. Own TSP verifier: at the end of an implementation it blocks for an explicit yes/no, then runs the full test-strategy flow over the change."
license: Apache-2.0
metadata:
  author: "alesierraalta"
  version: "0.1.0"
  requires_tsp: "0.6.6"
  scope: [common]
  auto_invoke: "At the end of an implementation, before the RDD/Gentle review or before declaring done: block for the explicit TSP-validation decision, then run the full test-strategy flow on yes"
---

# TSP Own Verifier

## Activation Contract

This is TSP's own verifier. It takes over the moment an implementation is finished — before the
Gentle AI RDD review when RDD is present, and immediately after the implementation when there is
no RDD. It does not hand the change to Gentle AI's verifier agents; TSP validates its own change
with its own flow.

The moment an implementation lands (code or behavior changed, work unit complete, ready for
review), run the Verification Gate below BEFORE declaring the work done, before the RDD review,
and before opening a PR or delivering.

## Verification Gate — the block

Stop. Do not declare done, do not start the RDD review, and do not deliver. Ask the user one
explicit question and do not advance until the user answers yes or no:

> ¿quieres hacer la validación del TSP?

Ask it through the host's blocking question mechanism so the turn genuinely waits for the human:
Pi `ask_user_choice`, Claude `AskUserQuestion`. This is a hard stop in the flow, the same way RDD
blocks: without an explicit answer the gate does not open and nothing proceeds. A yes/no is the
only acceptable answer; silence, a deferral, or the agent deciding on the user's behalf is a
closed gate.

- **yes** → open the gate and run the full test-strategy flow (below).
- **no** → record the decline as a one-line decision and proceed without validation. The decline
  is scoped to this change; it is not a kill switch and it does not disable TSP for later work.

Never skip the question because the change looks small, the tests look green, or the user seems
in a hurry. The gate is the product; the answer is the user's.

## On yes — the full test-strategy flow

Delegate the validation to the test-strategy skill and run it to completion over this change. Do
not truncate it to a quick check and do not stop at a green suite. The flow is the persisted
plan: inventory the change, sweep layers, rank targets, and execute each target through sandboxed
probes with executed evidence — no finding from reading alone. The verification reports what was
validated and what was NOT reachable, and every finding carries an executed evidence record per
`~/.claude/skills/test-strategy/references/evidence.md`.

Treat this as verification, not as a new implementation: fix only what the flow proves broken,
and re-run the affected targets until they are green on the final tree.

## Read Boundaries

- This gate decides whether to validate; it does not review the code and does not replace the
  RDD review, native review authority, receipts, consent envelopes, or delivery policy.
- The gate asks one question and runs one flow. It never invents a second gate, never answers
  for the user, and never records a decline as a success.
- Read-only outside the test-strategy flow this skill delegates to; the flow owns plan and
  evidence writes.
