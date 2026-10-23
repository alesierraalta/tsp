# Breakcheck readiness report

## Candidate identity

- Project/root: `rdd-plus` at worktree `../rdd-plus-worktrees/94-bounded-adversarial-gate`.
- Execution label: `breakcheck-feedback-parser-2026-09-17`.
- Candidate: `internal/feedback` (`internal/feedback/feedback.go`) and CLI surface `cmd/rdd-plus/main.go` (`runFeedback`), exercised at commit `069f70518ff0fc016b92599e41452d4aa65d6464`.
- Scope: `the feedback package (Parse, Record, Read, Summary) and its CLI surface at this commit; no diff is gated and no other package is claimed. The identity fields in this block were completed after the run to match the revised report template; probe measurements are unchanged.`
- Base: `N/A — no diff to gate; this run exercises the subsystem at the candidate commit.`
- HEAD: `069f70518ff0fc016b92599e41452d4aa65d6464`.
- Staged content: `none`.
- Unstaged content: `README.md`, `internal/assets/assets_test.go` (pre-existing and preserved; not part of this campaign).
- Untracked content: `assets/skills/breakcheck/SKILL.md`, `assets/skills/breakcheck/assets/readiness-report-template.md` (pre-existing and preserved; not part of this campaign), plus this report.
- Environment and isolation: `go build -o "${TMPDIR:-/tmp}/rdd-plus" ./cmd/rdd-plus` succeeded with exit 0. Every probe used a fresh scratch directory under `/tmp` and the real ledger was not written until the single required feedback submission. No full test suite, installer, network, or repository scratch file was used.
- Verifier evidence: `none for the campaign itself. T6 of odd/tasks/breakcheck.md is the acceptance criterion of issue #94; this report is also evidence for issue #96, which will change the recorder.`

## Budget

- Probe budget: `8 probes`.
- Time budget: `20 minutes wall clock`.
- Stop condition: Stop after eight isolated probes or at 20 minutes, whichever comes first; build and feedback submission are setup/closeout, not probes. Eight probes were completed, so no further probe was run.

## Risks and probes

| Risk | Probe | Expectation | Observed | Evidence |
| --- | --- | --- | --- | --- |
| Parse: unknown keys | P1 — submit a valid report with one unknown key | If an unknown key is not refused, this goes red; expect exit 2, the key named, and no ledger | `exit=2`; `feedback: unknown key(s): surprise`; `ledger_bytes=missing` | P1 below |
| Parse: CRLF, continuation, unicode, long values | P2 — submit CRLF input with a continued value, unicode, comments, and a 2048-character value | If accepted input is rejected or any value is mangled, this goes red; expect exit 0 and preserved values | `exit=0`; `paid=first line\ncontinued paid 🧪`; `reason=unicode café`; `guess_len=2048` | P2 below |
| Record: directory creation and first markdown mirror | P3 — submit into a missing scratch config directory | If telemetry, JSONL, or markdown is not created, this goes red | `exit=0`; ledger and markdown exist; `ledger_lines=1`; `markdown_header_count=1`; `markdown_sections=1` | P3 below |
| Record: append behaviour and existing markdown | P4 — submit two reports into one isolated config directory | If the first JSONL bytes are rewritten, the header repeats, or both records are lost, this goes red | `first_exit=0`; `second_exit=0`; `prefix=preserved`; `ledger_lines=2`; `markdown_header_count=1`; `markdown_sections=2` | P4 below |
| Record: existing ledger is not writable | P5 — submit to an existing mode-0444 ledger | If refusal is not nonzero or the ledger changes, this goes red | `exit=1`; `feedback: open .../run-feedback.jsonl: permission denied`; `ledger_unchanged=yes`; `markdown_exists=no` | P5 below |
| Read/Summary: empty, ordering/counts, unknown verdict, corrupt later line | P6 — read a missing ledger, then valid/unknown and valid/corrupt/valid ledgers | If missing is not empty, ordering/counts are wrong, an unknown verdict is silently omitted, or a corrupt line is silently skipped, this goes red | Missing ledger: `exit=0`, `no reports yet`. Unknown verdict: `exit=0`, total 3 but known verdict counts omit it and by-skill total is 2. Corrupt ledger: `exit=1`, `feedback: ledger line 2: invalid character ...` | P6 and smallest repro below; `FAIL` for the unknown-verdict inconsistency |
| CLI: default readback and `--config-dir` | P7 — use a scratch `HOME` for default lookup and a separate scratch `--config-dir` | If either path is ignored, this goes red | Both `exit=0`; default output contains `default-path` and paid 1; alternate output contains `config-dir` and ceremony 1 | P7 below |
| CLI: `--plan` override, `--template` no-write, refusal exit codes, verdict case | P8 — template with override, submit with override, missing file, and uppercase verdict | If template writes, override is ignored, or refusal statuses are wrong, this goes red | `template_config_exists_before_submit=no`; recorded plan `override.md`; missing file `exit=1`; `verdict: PAID` `exit=2` with the allowed verdicts named | P8 below |

## Evidence commands and observations

The following are the executed probe commands, all through `rtk run`; scratch paths are the observed paths printed by each command.

### P1 — unknown key

```sh
rtk run 'set -eu
scratch=$(mktemp -d "${TMPDIR:-/tmp}/breakcheck-p1.XXXXXX")
report="$scratch/report.md"; config="$scratch/config"
printf "%s\n" "ts: 2026-09-17T00:00:00Z" "repo: /synthetic" "plan: odd/tasks/breakcheck.md" "skill: breakcheck" "build: 069f705" "paid: parser" "cost: 1m" "reason: unknown-key probe" "verdict: paid" "surprise: reject-me" > "$report"
set +e
"${TMPDIR:-/tmp}/rdd-plus" feedback --config-dir "$config" --file "$report" > "$scratch/stdout" 2> "$scratch/stderr"; rc=$?
set -e
if [ -e "$config/telemetry/run-feedback.jsonl" ]; then ledger_state=$(wc -c < "$config/telemetry/run-feedback.jsonl"); else ledger_state=missing; fi
printf "scratch=%s\nexit=%s\nstderr=%s\nledger_bytes=%s\n" "$scratch" "$rc" "$(tr "\n" " " < "$scratch/stderr")" "$ledger_state"'
```

Observed: `scratch=/tmp/breakcheck-p1.D544qi`, `exit=2`, `stderr=feedback: unknown key(s): surprise`, `ledger_bytes=missing`.

### P2 — CRLF, continuation, unicode, long value

```sh
rtk run 'set -eu
scratch=$(mktemp -d "${TMPDIR:-/tmp}/breakcheck-p2.XXXXXX"); report="$scratch/report.md"; config="$scratch/config"
long=$(printf "x%.0s" $(seq 1 2048))
printf "# comment\r\n\r\nts: 2026-09-17T00:00:00Z\r\nrepo: /synthetic\r\nplan: odd/tasks/breakcheck.md\r\nskill: breakcheck\r\nbuild: 069f705\r\npaid: first line\r\ncontinued paid 🧪\r\ncost: 1m\r\nreason: unicode café\r\nverdict: paid\r\nguess: %s\r\nfreeform: comment-only lines are ignored\r\n# trailing comment\r\n" "$long" > "$report"
set +e
"${TMPDIR:-/tmp}/rdd-plus" feedback --config-dir "$config" --file "$report" > "$scratch/stdout" 2> "$scratch/stderr"; rc=$?
set -e
ledger="$config/telemetry/run-feedback.jsonl"
python3 -c "import json,sys;r=json.loads(open(sys.argv[1],encoding=\"utf-8\").readline());print(\"paid=\"+r[\"paid\"]);print(\"reason=\"+r[\"reason\"]);print(\"guess_len=\"+str(len(r[\"guess\"])))" "$ledger"
printf "scratch=%s\nexit=%s\nstdout=%s\nstderr=%s\n" "$scratch" "$rc" "$(tr "\n" " " < "$scratch/stdout")" "$(tr "\n" " " < "$scratch/stderr")"'
```

Observed: `scratch=/tmp/breakcheck-p2.sOOPY3`, `exit=0`, `stdout=recorded paid feedback for /synthetic`, `paid=first line` followed by `continued paid 🧪`, `reason=unicode café`, `guess_len=2048`.

### P3 — first record and mirror creation

```sh
rtk run 'set -eu
scratch=$(mktemp -d "${TMPDIR:-/tmp}/breakcheck-p3.XXXXXX"); report="$scratch/report.md"; config="$scratch/config"
printf "%s\n" "ts: 2026-09-17T00:00:00Z" "repo: /synthetic" "plan: odd/tasks/breakcheck.md" "skill: breakcheck" "build: 069f705" "paid: first record" "cost: 1m" "reason: create paths" "verdict: partly" > "$report"
"${TMPDIR:-/tmp}/rdd-plus" feedback --config-dir "$config" --file "$report" > "$scratch/stdout" 2> "$scratch/stderr"
ledger="$config/telemetry/run-feedback.jsonl"; markdown="$config/telemetry/run-feedback.md"
printf "scratch=%s\nexit=0\nstdout=%s\nledger_exists=%s\nledger_lines=%s\nmarkdown_exists=%s\nmarkdown_header_count=%s\nmarkdown_sections=%s\n" "$scratch" "$(tr "\n" " " < "$scratch/stdout")" "$(test -f "$ledger" && echo yes || echo no)" "$(wc -l < "$ledger")" "$(test -f "$markdown" && echo yes || echo no)" "$(grep -c "^# Run feedback$" "$markdown")" "$(grep -c "^## " "$markdown")"'
```

Observed: `scratch=/tmp/breakcheck-p3.fPhgib`, `exit=0`, `ledger_exists=yes`, `ledger_lines=1`, `markdown_exists=yes`, `markdown_header_count=1`, `markdown_sections=1`.

### P4 — append preservation

```sh
rtk run 'set -eu
scratch=$(mktemp -d "${TMPDIR:-/tmp}/breakcheck-p4.XXXXXX"); config="$scratch/config"
for n in 1 2; do verdict=paid; if [ "$n" = 2 ]; then verdict=ceremony; fi; printf "%s\n" "ts: 2026-09-1${n}T00:00:00Z" "repo: /synthetic" "plan: odd/tasks/breakcheck.md" "skill: breakcheck" "build: 069f705" "paid: record $n" "cost: 1m" "reason: append $n" "verdict: $verdict" > "$scratch/report$n.md"; "${TMPDIR:-/tmp}/rdd-plus" feedback --config-dir "$config" --file "$scratch/report$n.md" > "$scratch/out$n" 2> "$scratch/err$n"; if [ "$n" = 1 ]; then head -n1 "$config/telemetry/run-feedback.jsonl" > "$scratch/first-line"; fi; done
ledger="$config/telemetry/run-feedback.jsonl"; markdown="$config/telemetry/run-feedback.md"; head -n1 "$ledger" > "$scratch/after-first"; if cmp -s "$scratch/first-line" "$scratch/after-first"; then prefix=preserved; else prefix=changed; fi
printf "scratch=%s\nfirst_exit=0\nsecond_exit=0\nprefix=%s\nledger_lines=%s\nmarkdown_header_count=%s\nmarkdown_sections=%s\nsecond_stdout=%s\n" "$scratch" "$prefix" "$(wc -l < "$ledger")" "$(grep -c "^# Run feedback$" "$markdown")" "$(grep -c "^## " "$markdown")" "$(tr "\n" " " < "$scratch/out2")"'
```

Observed: `scratch=/tmp/breakcheck-p4.CtXg4T`, `first_exit=0`, `second_exit=0`, `prefix=preserved`, `ledger_lines=2`, `markdown_header_count=1`, `markdown_sections=2`, `second_stdout=recorded ceremony feedback for /synthetic`.

### P5 — read-only ledger

```sh
rtk run 'set -eu
scratch=$(mktemp -d "${TMPDIR:-/tmp}/breakcheck-p5.XXXXXX"); config="$scratch/config"; mkdir -p "$config/telemetry"; report="$scratch/report.md"
printf "%s\n" "ts: 2026-09-17T00:00:00Z" "repo: /synthetic" "plan: odd/tasks/breakcheck.md" "skill: breakcheck" "build: 069f705" "paid: readonly" "cost: 1m" "reason: readonly ledger" "verdict: paid" > "$report"
printf "%s\n" "{\"ts\":\"prior\",\"repo\":\"/synthetic\",\"plan\":\"p\",\"skill\":\"s\",\"build\":\"b\",\"paid\":\"p\",\"cost\":\"c\",\"reason\":\"r\",\"verdict\":\"paid\"}" > "$config/telemetry/run-feedback.jsonl"; chmod 0444 "$config/telemetry/run-feedback.jsonl"; before=$(sha256sum "$config/telemetry/run-feedback.jsonl" | awk "{print \$1}")
set +e
"${TMPDIR:-/tmp}/rdd-plus" feedback --config-dir "$config" --file "$report" > "$scratch/stdout" 2> "$scratch/stderr"; rc=$?
set -e
after=$(sha256sum "$config/telemetry/run-feedback.jsonl" | awk "{print \$1}")
printf "scratch=%s\nexit=%s\nstderr=%s\nledger_unchanged=%s\nmarkdown_exists=%s\n" "$scratch" "$rc" "$(tr "\n" " " < "$scratch/stderr")" "$(test "$before" = "$after" && echo yes || echo no)" "$(test -e "$config/telemetry/run-feedback.md" && echo yes || echo no)"'
```

Observed: `scratch=/tmp/breakcheck-p5.OO5KOi`, `exit=1`, `stderr=feedback: open /tmp/breakcheck-p5.OO5KOi/config/telemetry/run-feedback.jsonl: permission denied`, `ledger_unchanged=yes`, `markdown_exists=no`.

### P6 — readback and corrupt-line handling

```sh
rtk run 'set -eu
scratch=$(mktemp -d "${TMPDIR:-/tmp}/breakcheck-p6.XXXXXX"); empty="$scratch/empty"; unknown="$scratch/unknown"; corrupt="$scratch/corrupt"; mkdir -p "$unknown/telemetry" "$corrupt/telemetry"
set +e; empty_out=$("${TMPDIR:-/tmp}/rdd-plus" feedback --config-dir "$empty" 2> "$scratch/empty.err"); empty_rc=$?; set -e
printf "%s\n" "{\"ts\":\"1\",\"skill\":\"s\",\"verdict\":\"paid\",\"guess\":\"old\"}" "{\"ts\":\"2\",\"skill\":\"s\",\"verdict\":\"partly\",\"guess\":\"middle\"}" "{\"ts\":\"3\",\"skill\":\"s\",\"verdict\":\"mystery\",\"guess\":\"new\"}" > "$unknown/telemetry/run-feedback.jsonl"
set +e; unknown_out=$("${TMPDIR:-/tmp}/rdd-plus" feedback --config-dir "$unknown" 2> "$scratch/unknown.err"); unknown_rc=$?; set -e
printf "%s\n" "{\"ts\":\"1\",\"skill\":\"s\",\"verdict\":\"paid\"}" "{bad" "{\"ts\":\"3\",\"skill\":\"s\",\"verdict\":\"partly\"}" > "$corrupt/telemetry/run-feedback.jsonl"
set +e; corrupt_out=$("${TMPDIR:-/tmp}/rdd-plus" feedback --config-dir "$corrupt" 2> "$scratch/corrupt.err"); corrupt_rc=$?; set -e
printf "scratch=%s\nempty_exit=%s\nempty_output=%s\nunknown_exit=%s\nunknown_output=%s\ncorrupt_exit=%s\ncorrupt_stderr=%s\ncorrupt_stdout=%s\n" "$scratch" "$empty_rc" "$(printf "%s" "$empty_out" | tr "\n" "|")" "$unknown_rc" "$(printf "%s" "$unknown_out" | tr "\n" "|")" "$corrupt_rc" "$(tr "\n" " " < "$scratch/corrupt.err")" "$(printf "%s" "$corrupt_out" | tr "\n" "|")"'
```

Observed: `scratch=/tmp/breakcheck-p6.LnsGmH`; empty `exit=0`, output `no reports yet: run \`rdd-plus feedback --template\` after a run to start the ledger`; unknown `exit=0`, output `run feedback: 3 report(s)`, `paid: 1`, `partly: 1`, `ceremony: 0`, `s: 2 (paid 1, partly 1, ceremony 0)`, guesses `new`, `middle`, `old`; corrupt `exit=1`, stderr `feedback: ledger line 2: invalid character 'b' looking for beginning of object key string`, stdout empty.

Smallest reproducer for the unknown-verdict finding:

```sh
rtk run 'set -eu
scratch=$(mktemp -d "${TMPDIR:-/tmp}/breakcheck-p6-repro.XXXXXX"); mkdir -p "$scratch/telemetry"; printf "%s\n" "{\"ts\":\"1\",\"skill\":\"s\",\"verdict\":\"bogus\",\"guess\":\"x\"}" > "$scratch/telemetry/run-feedback.jsonl"; set +e; out=$("${TMPDIR:-/tmp}/rdd-plus" feedback --config-dir "$scratch" 2> "$scratch/err"); rc=$?; set -e; printf "scratch=%s\nexit=%s\nstderr=%s\noutput=%s\n" "$scratch" "$rc" "$(tr "\n" " " < "$scratch/err")" "$(printf "%s" "$out" | tr "\n" "|")"'
```

Observed: `scratch=/tmp/breakcheck-p6-repro.RSaNId`, `exit=0`, and `run feedback: 1 report(s)` with all three displayed verdict counts at zero and `s: 0 (paid 0, partly 0, ceremony 0)`, while guess `x` is displayed. This reproduces the silent inconsistency.

### P7 — default and config-dir readback

```sh
rtk run 'set -eu
scratch=$(mktemp -d "${TMPDIR:-/tmp}/breakcheck-p7.XXXXXX"); home="$scratch/home"; default="$home/.claude"; alt="$scratch/alt"; mkdir -p "$default/telemetry" "$alt/telemetry"
printf "%s\n" "{\"ts\":\"d\",\"skill\":\"s\",\"verdict\":\"paid\",\"guess\":\"default-path\"}" > "$default/telemetry/run-feedback.jsonl"; printf "%s\n" "{\"ts\":\"a\",\"skill\":\"s\",\"verdict\":\"ceremony\",\"guess\":\"config-dir\"}" > "$alt/telemetry/run-feedback.jsonl"
set +e; HOME="$home" "${TMPDIR:-/tmp}/rdd-plus" feedback > "$scratch/default.out" 2> "$scratch/default.err"; default_rc=$?; "${TMPDIR:-/tmp}/rdd-plus" feedback --config-dir "$alt" > "$scratch/alt.out" 2> "$scratch/alt.err"; alt_rc=$?; set -e
printf "scratch=%s\ndefault_exit=%s\ndefault_stderr=%s\ndefault_output=%s\nconfig_dir_exit=%s\nconfig_dir_stderr=%s\nconfig_dir_output=%s\n" "$scratch" "$default_rc" "$(tr "\n" " " < "$scratch/default.err")" "$(tr "\n" "|" < "$scratch/default.out")" "$alt_rc" "$(tr "\n" " " < "$scratch/alt.err")" "$(tr "\n" "|" < "$scratch/alt.out")"'
```

Observed: `scratch=/tmp/breakcheck-p7.MpxBhP`; both exits were 0. Default output read `default-path` and `paid: 1`; `--config-dir` output read `config-dir` and `ceremony: 1`.

### P8 — template, plan override, and refusal codes

```sh
rtk run 'set -eu
scratch=$(mktemp -d "${TMPDIR:-/tmp}/breakcheck-p8.XXXXXX"); config="$scratch/config"; "${TMPDIR:-/tmp}/rdd-plus" feedback --config-dir "$config" --template --plan override.md > "$scratch/template" 2> "$scratch/template.err"; template_config_exists=$(test -e "$config" && echo yes || echo no)
printf "%s\n" "ts: 2026-09-17T00:00:00Z" "repo: /synthetic" "plan: original.md" "skill: breakcheck" "build: 069f705" "paid: override" "cost: 1m" "reason: override" "verdict: paid" > "$scratch/valid.md"; "${TMPDIR:-/tmp}/rdd-plus" feedback --config-dir "$config" --file "$scratch/valid.md" --plan override.md > "$scratch/submit.out" 2> "$scratch/submit.err"; python3 -c "import json,sys;r=json.loads(open(sys.argv[1],encoding=\"utf-8\").readline());print(r[\"plan\"])" "$config/telemetry/run-feedback.jsonl" > "$scratch/plan"
printf "%s\n" "ts: 2026-09-17T00:00:00Z" "repo: /synthetic" "plan: p" "skill: breakcheck" "build: 069f705" "paid: x" "cost: 1m" "reason: uppercase" "verdict: PAID" > "$scratch/uppercase.md"; set +e; "${TMPDIR:-/tmp}/rdd-plus" feedback --config-dir "$config" --file "$scratch/missing.md" > "$scratch/missing.out" 2> "$scratch/missing.err"; missing_rc=$?; "${TMPDIR:-/tmp}/rdd-plus" feedback --config-dir "$config" --file "$scratch/uppercase.md" > "$scratch/uppercase.out" 2> "$scratch/uppercase.err"; uppercase_rc=$?; set -e; printf "scratch=%s\ntemplate_plan=%s\ntemplate_config_exists_before_submit=%s\nsubmit_exit=0\nrecorded_plan=%s\nmissing_exit=%s\nmissing_stderr=%s\nuppercase_exit=%s\nuppercase_stderr=%s\n" "$scratch" "$(grep "^plan: " "$scratch/template")" "$template_config_exists" "$(cat "$scratch/plan")" "$missing_rc" "$(tr "\n" " " < "$scratch/missing.err")" "$uppercase_rc" "$(tr "\n" " " < "$scratch/uppercase.err")"'
```

Observed: `scratch=/tmp/breakcheck-p8.yUz4vr`, `template_plan=plan: override.md`, `template_config_exists_before_submit=no`, `submit_exit=0`, `recorded_plan=override.md`, missing file `exit=1` with `feedback: open .../missing.md: no such file or directory`, uppercase verdict `exit=2` with `feedback: verdict "PAID" must be one of: paid, partly, ceremony`.

## Findings

| ID | Severity by consequence | Evidence and smallest reproducer | Disposition |
| --- | --- | --- | --- |
| F-01 | limited — a manually corrupted/legacy ledger line is counted in the total and recent guesses but silently omitted from displayed verdict and by-skill counts, producing misleading telemetry without changing the JSONL bytes | P6 smallest reproducer above: one JSONL line with `"verdict":"bogus"`; exact observed output is `run feedback: 1 report(s)` with `paid: 0`, `partly: 0`, `ceremony: 0`, `s: 0 (...)`, and guess `x` | FAIL |

No other suspected defect survived a smallest-reproducer rerun. P1, P2, P3, P4, P5, P7, and P8 passed their stated invariants; P6 passed empty-ledger, ordering, and corrupt-line refusal but failed the unknown-verdict consistency invariant.

## Omissions

| Risk omitted | Reason | Residual risk |
| --- | --- | --- |
| Repeated keys | Eight-probe cap; unknown-key refusal was higher-value for schema safety. | Duplicate fields may be accepted with last/concatenated-value semantics not measured. |
| Whitespace-only or empty required values; comment-only input | Eight-probe cap; existing focused tests cover missing required fields, but this campaign did not rerun tests. | CLI behaviour for whitespace-only required values and an otherwise comment-only file remains unmeasured. |
| A value that looks like a new key | Eight-probe cap; the selected continuation probe used ordinary continuation text. | Key-looking prose may be interpreted as a new field and refused or reassigned. |
| Partial writes, interrupted writes, concurrent appends, and markdown failure after a successful ledger append | These require fault injection or concurrency and were dropped to keep the campaign within 20 minutes and avoid adding harness code. | Ledger/markdown divergence and interleaving under interruption or concurrency remain unmeasured. |
| Independently pre-existing or malformed markdown mirror | P3/P4 covered missing and generated-existing mirrors, not an arbitrary pre-existing file. | Existing mirror contents may interact unexpectedly with append rendering. |
| Direct package calls separate from the CLI seam | The user-required candidate includes the CLI surface; the eight probes exercised it end to end. | Package-only callers may expose different error handling. |
| Full test suite and unrelated repository checks | Explicitly prohibited by the method for this campaign. | Unrelated regressions are outside this evidence. |

## Readiness disposition

- Status: `N/A`.
- Rationale: `This report is evidence for issue #96, not a merge gate; there is no diff to gate and no readiness claim is being made.`
- Accepted risks: `none; F-01 and every omission remain visible and are not repaired or silently accepted.`
- Accepting authority: `none — N/A by explicit user instruction.`
- Residual risk: `F-01 is a limited silent-summary inconsistency for an unknown verdict already present in a ledger; omitted parser, fault, concurrency, and mirror cases remain unmeasured.`

## Repository state verification

Command: `rtk git status --short --untracked-files=all`

Observed:

```text
 M README.md
 M internal/assets/assets_test.go
?? assets/skills/breakcheck/SKILL.md
?? assets/skills/breakcheck/assets/readiness-report-template.md
?? docs/testing/breakcheck-feedback-parser.md
```

`rtk git status --short --ignored odd/` observed `!! odd/`. The requested "only the new report" state was not present because the first four paths were pre-existing changes; they were preserved and not edited by this campaign.

## Feedback record

- Result: `recorded exactly once after successful submission: exit=0; stdout="recorded partly feedback for ../rdd-plus-worktrees/94-bounded-adversarial-gate"; stderr empty.`
- Command: `rtk run 'feedback_file="${TMPDIR:-/tmp}/breakcheck-feedback-$(date +%s).md"; "${TMPDIR:-/tmp}/rdd-plus" feedback --template > "$feedback_file"; fill only the existing fields; "${TMPDIR:-/tmp}/rdd-plus" feedback --file "$feedback_file"'` (the actual fill used only `ts`, `repo`, `plan`, `skill`, `build`, `paid`, `cost`, `reason`, `verdict`, `guess`, and `freeform`; no `--config-dir` was passed on submit).
- Ledger/reference: `~/.claude/telemetry/run-feedback.jsonl` (default config directory); verdict recorded was `partly`. The required feedback submission succeeded, so it is claimed as recorded.
