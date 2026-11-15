# Baseline

`tsp bench gate --baseline bench/baseline` compares a fresh run against the aggregate recorded here.

The gate refuses the pair when the run and this baseline recorded a different model, runner, corpus or case
set, because subtracting one instrument's number from another's produces a delta nobody can attribute. The
refusal names the field that differs. `--skill-versions` is the one deliberate exception, for reading two
skill versions on the same instrument.

A baseline is only honest when it measured the same corpus with the same instrument as the run being
compared. This one does, and the instrument is written down:

| field | value |
|---|---|
| cases | `bench/cases/h*` (23 real upstream-bug cases, one planted defect each) |
| runner | `pi` |
| model | `openai-codex/gpt-6-luna` |
| agent config | `bench` (the test-strategy skill only) |
| runs per case | 2 |
| skill version | 0.7.25 (the embedded `test-strategy` the bench config carries) |
| commit measured | `22d8145b` (clean tree; the scorer is reproducible from it) |
| date | 2026-11-14 |
| caught | 37 of 45 valid defect-runs, recall 0.822; all 8 misses `seen_tests_missed` (h06, h10 and h29 missed in both runs, h18 and h25 in one) |
| false positives | 0 |
| cost | $0.155, $0.0042 per caught defect |
| tokens | 865,261 uncached input, 4,329,728 cache read, 49,918 output (4,222 reasoning); cache hit rate 0.833 |
| invalid | 1 run: `h03` run 1 (excluded from recall rather than counted as a miss) |

The previous baseline (2026-10-26, commit `9eda2611`, skill 0.7.6, one run per case, an older corpus
digest) caught 16 of 22. It is not a reading of this instrument, so the two numbers are not a delta.

The workflow installs pi and forwards the provider keys pi already understands
(`ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, `OPENCODE_API_KEY`, `OPENROUTER_API_KEY`, `GEMINI_API_KEY`) from
repository secrets, so nothing bespoke is invented for CI. Two honest limits: with no key it prints a
notice instead of a green run, and this baseline's model is subscription-backed, so a hosted run has to
set `TSP_BENCH_MODEL` to a key-backed model — and then it is read against the recall floor only, because
numbers from a different instrument are not comparable.

Re-record this baseline when the corpus changes or the instrument changes; a comparison across different
instruments is noise wearing a number.
