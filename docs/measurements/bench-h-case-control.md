# H-case control: can the corpus see the change at all?

`measure-change` landed, and then the `test-strategy` payload changed: two rules entered
`assets/test-strategy/assets/test-plan-template.md` and `metadata.version` moved to `0.7.12`. A payload change
owes a reading. This page records the one this machine could produce, and what it cannot say.

## Method

- The 23 `h*` cases the checked-in baseline covers, same case set, so the corpus digest matches
  (`sha256:7f069342a7a56f98` on both sides).
- One run per case, arm `bench` (only the embedded `test-strategy` installed), runner `agy`, model
  `gemini-3.8-flash-low`, concurrency 4, skill version `0.7.12`, budget ceiling $8, 2026-10-30.
- Not the baseline's instrument, and not by choice. `openai-codex/gpt-6-luna` failed every run with
  `"code": "invalid_refresh_token"`; the runner's default model failed with `No API key found for
  opencode`; `claude` exited 1 with an empty message. `agy` was the only runner that produced a run,
  and it is free, so the ceiling never bound: **$0.00 spent of the $5–15 approved**.

## Result

```
instrument                     recall_caught   cost     cases   ledger rows
gpt-6-luna     (baseline)      0.727           0.094    23      not recorded
gemini-3.8-flash-low (today)   0.913           0.000    23      0 of 23
```

`tsp bench gate --baseline bench/baseline --max-regression 0.05 bench/results/20261030-161642` printed
`recall 0.913 over 23 defect(s), cost 0.0000` and exited 0.

Two cases produced no witness: `h06-ent-like-pattern-escape`, `h18-gitleaks-reader-eof-with-bytes`.

## What it means, and what it does not

- **The 0.913 against 0.727 is a model difference, not an improvement.** Two instruments, one comparison,
  and the gate drew it without a word: `internal/bench/gate.go` compares `recall_caught` and never checks
  the model, the runner or the skill version, while `compare` refuses exactly those mismatches
  (`internal/bench/compare.go:82-108`). Filed as #327, with today's run as its live demonstration.
- **The change under review cannot appear in this number.** The two rules are prose in the plan template,
  which reaches `tsp plan init` output, not the agent prompt. All 23 runs also went plan-less — `0 of 23`
  recorded a ledger row — so nothing here exercises the ledger at all. This reading measures detection on
  23 real defects, not the rules that entered the template.
- **What it does establish:** the payload at `0.7.12` loads and the flow runs end to end; the corpus digest
  is unchanged by the payload edit, as it should be; and with this model, two cases in the family yield no
  witness.

## Reading it again, properly

The comparable control needs the baseline's own instrument: refresh the `codex` login, then run the same 23
cases with `--model openai-codex/gpt-6-luna` and compare with `tsp bench compare` (which will refuse the
mismatch if the login is still stale, rather than report a delta). Until then the honest statement is
"no regression signal, and no signal about the change either".
