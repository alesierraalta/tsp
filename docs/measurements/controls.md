# Control cases: what the harness invents when the code is already correct

A recall number says nothing about precision. Both control cases in the corpus hold code that is
already correct: a harness that reports at all there is inventing, and a harness that reports nothing has
proved nothing unless it did the work. This page records which of the two happened.

## Method

- Cases `c01-clean-allocate` (Node, `node --test`) and `g05-clean-backoff` (Go, `go test ./...`), both with
  `"control": "clean"` in `KEY.json` and `"defects": []`.
- One run per case, arm `bench` (only the `test-strategy` skill installed), model
  `openai-codex/gpt-6-luna`, budget $0.60, 2026-10-25.
- A run counts only when its plan artifact was found at `docs/testing/test-plan.md`. The first attempt
  proved the trap: both runs reported nothing **and wrote no plan at the default path** (one wrote
  `TEST_PLAN.md` instead), which is an abstention, not a clean result.

## Result

```
case                  plan   findings raised   invented
c01-clean-allocate    found  0                 0
g05-clean-backoff     found  0                 0
```

`tsp bench` renders it as `controls: 2 runs · inventions on controls: 0 finding rows · no plan: 0 runs`.

## What it means, and what it does not

- Two controls with a plan and zero findings each is **no evidence of invention** on these two fixtures.
  It is not a precision figure for the corpus: the corpus has 165 cases and 252 defects, and precision
  over defect cases still needs the 45 runs with pending adjudication resolved (`precision: none` in the
  summary is that queue, not a zero).
- The honest statement today is narrow: on a correct Node library and a correct Go library, the flow
  planned the layers, ran, and raised nothing. Wider claims need more control cases and adjudicated
  defect runs, which is what the remaining bench work owes.
