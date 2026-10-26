# Baseline

`tsp bench gate --baseline bench/baseline` compares a fresh run against the aggregate recorded here.

A baseline is only honest when it measured the same corpus with the same instrument as the run being
compared. This one does, and the instrument is written down:

| field | value |
|---|---|
| cases | `bench/cases/h*` (23 real upstream-bug cases, one planted defect each) |
| runner | `pi` |
| model | `openai-codex/gpt-6-luna` |
| agent config | `bench` (the test-strategy skill only) |
| runs per case | 1 |
| commit measured | `9eda2611` (clean tree; the scorer is reproducible from it) |
| date | 2026-10-26 |
| caught | 16 of 22 valid defect-runs, recall 0.727 |
| cost | $0.094 |
| invalid | 1 case: `h04-testify-notcontains-noncontainer-message` (its own suite is red in this environment, so it is excluded from recall rather than counted as a miss) |

The workflow restores a pi home from the `TSP_PI_HOME_B64` secret — your own pi credentials, not a
vendor API key — or uses a self-hosted runner that is already logged in, and prints a notice instead of
a green run when neither exists.

Re-record this baseline when the corpus changes or the instrument changes; a comparison across different
instruments is noise wearing a number.
