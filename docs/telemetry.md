# Usage telemetry

Telemetry answers one question locally: what did tsp actually do, and what did each run cost? It
is a ledger of counts, enums, durations, versions and salted pseudonyms — never a copy of anything
you wrote.

Telemetry is **opt-in and off by default**. With it off, tsp writes no directory, no salt and no
ledger; `tsp telemetry record` validates what a host sends and then keeps nothing.

## Enable and disable

```sh
tsp feature enable telemetry     # start recording
tsp feature disable telemetry    # stop recording; nothing is written while off
tsp feature list                 # show the toggle
tsp feature enable telemetry --preview   # what turning it on records
```

The gate is read **once, before the subcommand runs**. That is why `tsp feature enable telemetry`
is not itself recorded (the run started opted out), and why `tsp feature disable telemetry` is
recorded (the run started opted in). Disabling never deletes a ledger that already exists.

## Where it is written

One append-only file:

```
<state root>/telemetry/events.jsonl
```

`<state root>` honours `TSP_HOME`, then `TPP_HOME`, then `RDD_PLUS_HOME`, and defaults to the XDG
config directory's `tsp` folder. One JSON object per line, `schema: 1`, appended under an exclusive
file lock in a single write, so concurrent tsp processes never interleave lines. The file is `0600`
inside a `0700` directory.

The same directory holds the salt the pseudonyms are sealed with (`<state root>/telemetry/.salt`),
created on first use. The salt never leaves the machine: without it a pseudonym cannot be reversed.
The ledger and the salt are never publishable.

A telemetry failure never changes what a command does. The command's stdout and exit code are
untouched and the failure costs exactly one stderr line:

```
tsp telemetry: <what failed>
```

A ledger that cannot be written therefore costs a line, not a run.

## The no-content rule

Recorded: counts, enums, durations, versions, flag **names**, and salted pseudonyms.

Never recorded: code, prompts, finding text, plan text, flag **values**, positional arguments,
paths, repository names, hostnames, session ids, model output, or anything else free-form. Every
input is either an integer, a member of a closed enum, or an identifier matching a fixed pattern.
A field nobody measured is **absent**, never zero: an absent `compactions` means "not measured", a
zero means "measured, none".

`tsp telemetry record` enforces this with a closed shape: an unknown key, free text, a negative
number, an out-of-bounds map or array, or trailing data after the JSON object is refused with exit
2 and no `host_run` is written. The refusal still produces its own `command` event (name
`telemetry`, sub `record`, exit 2), because every tsp invocation records what it did; refusing the
host event never refuses the record of the attempt.

## Envelope

Every line carries the same envelope.

| Field | Type | Meaning |
|---|---|---|
| `ts` | string | RFC 3339 UTC timestamp with nanoseconds. |
| `schema` | int | Line-format version; currently `1`. A reader refuses a version it does not know. |
| `source` | string | Who produced the event: `tsp`, `pi`, `claude` or `opencode`. |
| `kind` | string | `command`, `host_run`, and the domain kinds (`check`, `finding`, `plan`). |
| `run_id` | string, absent | The run this event belongs to, minted by the host and passed in `TSP_RUN_ID`. Kept only when it matches `^[A-Za-z0-9._:-]{1,128}$`; absent when unset or outside that pattern — never an empty string. |
| `version` | string | The tsp release that wrote the line. |
| `commit` | string | The revision behind that build (`<sha>` or `<sha>+dirty`, `unknown` when the build carries none). |
| `skill` | string | Identity of the embedded test-strategy skill, `<name> <version>`, or `unknown`. |
| `os` | string | `GOOS` of the writer. |
| `arch` | string | `GOARCH` of the writer. |
| `cpus` | int | Logical CPUs on the machine. |
| `repo` | string, absent | Salted pseudonym of the repository's canonical common git directory. Absent outside a repository or when git cannot answer. |
| `machine` | string, absent | Salted pseudonym of the hostname. Absent when the host will not say. |
| `session` | string, absent | Salted pseudonym of the host session id. The raw value is never written. |

Pseudonyms have the form `<prefix>-<12 hex chars>`, for example `session-3f9a1c40b7de`. The same
value yields the same pseudonym on one machine and a different one on any other, because the salt
is local.

## `command` events

One per tsp invocation, written by `tsp` itself.

| Field | Type | Meaning |
|---|---|---|
| `name` | string | The dispatched subcommand: `check`, `plan`, `telemetry`, `version`, … An argument the CLI did not dispatch is recorded as `unknown`, never as the argument itself. |
| `sub` | string, absent | The nested subcommand, and only when it is on the explicit documented allow-list for `name` (`plan check` records `check`). A usage prose word, positional path or run slug is never a `sub`. |
| `flags` | array of string, absent | The flag **names** given that the command's own usage and help texts document, in order, deduplicated. `--path=secret/x.md` contributes `path`; the value and any positional argument never appear, and a dash-prefixed value such as `--file -private` contributes `file` only. Scanning stops at `--`. A value that is itself a documented flag word can be recorded as a flag name; no user text can. |
| `exit` | int | The process exit code the command returned. |
| `duration_ms` | int | Wall-clock milliseconds for the subcommand. |

A refused or failed command is recorded exactly like a successful one, with its own exit code: the
failure rate is `exit != 0` over all `command` events, and a command that failed is often the most
interesting line in the ledger.

## `host_run` events

One per agent run. Two adapters write them:

- **Pi** (`assets/hosts/pi/tsp.ts`): when the extension detects a TSP run it mints a `run_id`
  (`tsp-<epoch ms>-<hex>`), exports it as `TSP_RUN_ID` to every tsp it spawns, accumulates the
  usage of each assistant message, tool calls by name, tool errors and compactions, and sends one
  event through `tsp telemetry record` on settle. A declined or dismissed offer is sent with
  `offer` set and no `run_id`.
- **Claude Code** (the `tsp gate` Stop hook): when the session transcript evidences a TSP run, the
  gate reads the transcript usage (`input_tokens`, `output_tokens`, `cache_read_input_tokens`,
  `cache_creation_input_tokens`, deduplicated by message id), tool calls by name, tool errors,
  model and wall time, and notes one event with source `claude`. A per-session cursor, named by a
  hash, makes a later Stop count only the turns added since. The gate's decision, stdout and exit
  are unchanged. `compactions`, `subagents`, `first_tsp_ms`, `cost_usd` and `offer` stay absent.

Other hosts send the same shape:

```sh
printf '%s' "$event" | tsp telemetry record
```

Input:

| Field | Type | Meaning |
|---|---|---|
| `source` | string, required | `pi`, `claude` or `opencode`. Any other value is refused. |
| `run_id` | string, absent | Identifier matching `^[A-Za-z0-9._:-]{1,128}$`. Absent when the host mints none. |
| `session` | string, absent | Raw session id matching the same pattern; stored only as a salted pseudonym. |
| `host` | object, absent | The run's usage, below. |

`host`:

| Field | Type | Meaning |
|---|---|---|
| `model` | string, absent | Model name matching `^[A-Za-z0-9._:/@-]{1,128}$`. |
| `turns` | int ≥ 0, absent | Agent turns in the run. |
| `tool_calls` | int ≥ 0, absent | Tool calls made. |
| `tool_errors` | int ≥ 0, absent | Tool calls that returned an error. `0` is a measurement; absent is not. |
| `compactions` | int ≥ 0, absent | Context compactions. |
| `subagents` | int ≥ 0, absent | Delegated subagent sessions. |
| `wall_ms` | int ≥ 0, absent | Wall time of the whole run. |
| `first_tsp_ms` | int ≥ 0, absent | Time from run start to the first tsp call. |
| `tokens` | object, absent | Run totals, below. |
| `cost_usd` | number ≥ 0, absent | Reported cost. |
| `per_turn` | array of tokens, ≤ 5000, absent | One entry per turn; each entry carries only the counters the host measured. |
| `tools` | object name → int ≥ 0, ≤ 200, absent | Calls per tool name. Each name matches `^[A-Za-z0-9_.:-]{1,64}$`. |
| `offer` | string, absent | What the host offered: `accepted`, `declined`, `aborted` or `none`. Any other value is refused. |

Token counters (`tokens` and each `per_turn` entry):

| Field | Type | Meaning |
|---|---|---|
| `input` | int ≥ 0, absent | Input tokens. |
| `output` | int ≥ 0, absent | Output tokens. |
| `cache_read` | int ≥ 0, absent | Tokens read from the prompt cache. |
| `cache_write` | int ≥ 0, absent | Tokens written to the prompt cache. |

Cache hit rate is derived by the reader as `cache_read / (input + cache_read + cache_write)`, and
only from the counters present: the ledger stores raw counts, never ratios, so a reader that wants
a different denominator can compute it.

### Exit codes of `telemetry record`

| Code | Meaning |
|---|---|
| 0 | Accepted. Silent on stdout and stderr. With telemetry off, accepted and nothing kept. |
| 2 | Refused: a usage error, or an event outside the closed shape. No `host_run` was written; the invocation's own `command` event remains. |

A refusal names the field, never the value that was sent:

```
tsp telemetry record: model must match ^[A-Za-z0-9._:/@-]{1,128}$
```

## Domain events

Written by tsp itself from the results it already computed. Counts are absent when not measured.

`check` (one per `tsp check`):

| Field | Meaning |
|---|---|
| `decision` | The gate level check settled on. |
| `exit` | Check's exit code. |
| `reason` | Closed category: `witness`, `pins_nothing`, `no_witness`, `no_run`, `waived`, `opt_out`, `no_source`, `plan_open`, `plan_done`, `plan_micro`, `error`. |
| `suite` | The closed runner name that ran (`go test`, `pytest`, `node --test`, `jest`, `vitest run`, `cargo test`, `npx jest`, `npx vitest run`), or `other`; never the caller's command or a path. |
| `runners` | The runners the changed languages call for. A runner here that `suite` is not names a suite that did not run. |
| `languages` | Extensions (without the dot) of the changed production files. |
| `files`, `lines` | Changed production files, and their added plus removed lines. |
| `test_files`, `red`, `pins_nothing`, `new_code` | Changed test files; new tests failing on the unmodified code; new tests passing on it; witnesses only the change's new code can run. |
| `waiver`, `opt_out` | Present and `true` when the verdict came from a waiver or an opt-out. |

`finding` (one per `tsp findings record`, `verdict`, `check`): `action` (`record`, `verdict`, `check`),
`class` (a known defect family only; a refused class is never kept), `verdict`, `count`.

`plan` (one per `tsp plan init`, `micro`, `add-finding`, `add-evidence`, `admit`, `check`): `action`,
`findings`, `evidence`, `problems`.

## Reading the ledger

`tsp telemetry stats [--json]` groups the events by tsp version and skill identity and prints:
events and distinct runs; per subcommand its count, nonzero exits and their rate, and p50/p95
duration (nearest rank); the check decision distribution; the blocked rate (check `exit` 2 or
`reason` `error`); host totals of input, output, cache read and cache write tokens and the cache hit
`cache_read / (input + cache_read + cache_write)` over the host runs that measured all three; turns
and tool calls per host run; the offer distribution; and tokens per pinned red test (host tokens of
the runs whose check events have `red > 0`, over their summed `red`). A nonzero exit is not always
an error: check and redcheck answer with exit 1. Malformed lines (not JSON, not an object, or a
`schema` other than 1) are counted and reported, never dropped silently. Reading works while the
feature is off.

`tsp telemetry export --format jsonl|csv` writes the valid lines verbatim, or as CSV with one column
per dotted field (sorted header; arrays and the `tools` map are JSON in their cell), for correlation
in any external tool. An unknown format exits 2.

The file itself is newline-delimited JSON, so any reader works:

```sh
jq -c 'select(.kind == "command") | {name, sub, exit, duration_ms}' \
  "$(tsp status --json | jq -r .stateRoot)/telemetry/events.jsonl"
```
