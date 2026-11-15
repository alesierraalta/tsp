# Pi TSP extension: live-session proof

Pinned: Pi 1.5.0 (`pi --version`), extension at `assets/hosts/pi/tsp.ts` on `main` at `cd14d69f` (after PR #352, which shipped #345-#348), `tsp` built from the same tree. Harness: [assets/hosts/pi/live/](../assets/hosts/pi/live/README.md). The model is scripted, so every result below comes from Pi's real runtime and the real extension, not from a mock of either. A first proof ran on Pi 1.4.0 against an earlier implementation of the same issues; it is superseded by this one.

## What was observed

| Case | Result |
| --- | --- |
| An `edit`/`write` of non-test source produces the offer | pass |
| Answering "Sí" sends "haz el TSP" as a user message in the same session | pass |
| Answering "No" is remembered for those files; a different file is offered again | pass |
| The status line shows `tsp: ask` once the offer is raised (`extension_ui_request` `setStatus` with `statusKey`, `statusText`) | pass |
| `/tsp run` typed in the session sends "haz el TSP" and starts the run, with no offer | pass |
| A "No" survives resuming the stored session with `--continue` (second Pi process) | pass; the same case fails on the extension before #346 (offered twice) |
| Edits made during the TSP run itself produce no second offer | pass |
| No offer after an aborted run (`agent_settled.aborted = true`) | pass |
| No offer, and no `tsp` call at all, with `.no-testing-gate` | pass |
| No offer and no TSP request without a UI (`--mode json`) when the verdict is `ask` | pass |
| A `required` verdict (`.tsp.json` `"gate":"always"`) runs TSP without asking | pass |
| Source changed by `sed -i` through the `bash` tool produces the offer | pass (gap closed by #345; it was a known gap) |
| Source changed by another process during the turn (what a delegated worker does) produces the offer | pass (gap closed by #345) |
| A file that was already dirty before the turn and not touched by it produces no offer | pass |
| A `required` verdict in a session without a UI (`--mode json`) puts "haz el TSP — …" in front of the model exactly once | pass (gap closed by #347; fails on the extension before it) |
| A Gentle session (a Gentle-owned tool registered, no `.tsp.json`) runs TSP without asking, with a UI | pass (fails on the extension before #418) |
| A Gentle session without a UI (`--mode json`, no `.tsp.json`) puts "haz el TSP — …" in front of the model exactly once | pass (fails on the extension before #418) |
| A TSP loop in a session without a UI (`--mode json`) sends the fix continuation as a user message, and Pi runs it as one more request of the same run, with no dialog | pass (closed by #471) |
| A TSP loop in a session without a UI (`--mode json`) whose tree is uncommitted asks the agent to commit, and records the round after the commit it asked for | pass (closed by #471) |

No gap is open. The three rows that were known gaps (#345 twice, #347) carried a `gap` marker in the harness, which fails as `GAP CLOSED` when the wanted behaviour appears; each marker was removed with the change that closed it.

## Events and payloads observed

- Event types reaching an RPC client: `session`, `agent_start`, `turn_start`, `message_start`/`update`/`end`, `tool_execution_start`/`update`/`end`, `turn_end`, `agent_end`, `agent_settled`, `extension_ui_request`, `response`.
- `agent_settled` carries `{ type, aborted }`. `aborted` is `true` after an `abort` command.
- The offer is `extension_ui_request` with `method: "select"`, `id`, `title` (`"¿Quieres hacer TSP?"` followed by the debt line) and `options` (`["Sí","No"]`). Other methods seen: `notify` (fields `message`, `notifyType`).
- The extension raises the question from inside its `agent_settled` handler, so the RPC client receives the `select` request in the same millisecond as, and before, the `agent_settled` event; the handler stays blocked until the client answers.
- In `--mode json` the same lifecycle events fire (`before_agent_start`, `agent_before_settle`, `agent_settled`) with `hasUI = false`.
- A message an extension adds at `agent_before_settle` (a `custom_message` entry with `continue: true`) makes Pi send one more request to the model, but Pi does not announce it as a message event: the harness sees it through what the scripted model received.

## Sessions without a UI

Since #347 the extension acts there in one case only: when `tsp check --json` answers `required` (a `.tsp.json` `"gate":"always"`, or a verified Gentle session: since #418 the extension passes that observation to every check it runs, so a Gentle session without a declared gate is `required`) for the source changed in a completed turn, it adds one `custom_message` ("haz el TSP — <why>") with `continue: true` at `agent_before_settle`, once per user turn. It does not ask when the verdict is `ask` (that is the human's question), does nothing in a delegated child, with `.no-testing-gate`, after an aborted or failed run, or when the turn changed no source. The check is still run in those sessions, so `tsp` is called once per turn that changed source.

Since #471 that continuation's run is followed by the loop as well, whenever the `auto-run` feature is on: at `agent_settled` the extension asks `tsp findings record` and `tsp findings next --json` and sends the same fix / round / record continuation the interactive loop sends, as a user message. Pi defers a message sent from `agent_settled` until every settled handler has finished and then runs it, so the print-mode session keeps going instead of exiting. Nothing is rendered — no toast, no status line, no panel — `done`, `stop` and `error` end the loop normally, and the same round and no-progress bounds apply. Because the loop judges commits and refuses an uncommitted tree (`internal/findings/loop.go` `errDirtyRecord`/`errDirtyNext`), a headless session that left its tests uncommitted is asked to commit them together with the fix they pin and change nothing else, naming the paths git still sees; the turn that asks for records the round and asks `next` again, and the request is bounded at one per settle and two per session, after which the loop ends. A headless run leaves no status line, so two things the chat would have shown are session entries instead: the loop's own answer as `tsp-loop` (`next`, `round`, `max_rounds`, `open`), and a review that failed for any reason other than the dirty-tree refusal as `tsp-loop-error` (the command, its exit code and the tail of what it printed). Reading a product-arm transcript therefore no longer needs the throwaway findings registry, which lives in the per-run `agent-config` copy and is removed by `finish()` (`internal/bench/run.go:1048`). With `auto-run` off a session without UI drives nothing: no boundary continuation and no loop. The live harness runs both cases (`a TSP loop without a UI sends the fix continuation and Pi runs it` and `…asks the agent to commit, and records after it`) with a scripted `tsp` answering `findings record`/`findings next`, because the sandbox has no plan to judge; the real binary's answers are covered by `assets/hosts/pi/tsp.test.mjs`.

## The blind spot and whether an event closes it

No Pi event reports what a shell command changed: `tool_result` and `tool_execution_end` for `bash` carry the command and its output only. Delegated workers run as separate `pi --mode rpc` processes, the extension disables itself there, and the parent sees only the subagent tool result. Edits made inside a `codemode` script do reach `tool_result` as nested `edit` calls, so that route is not a gap.

The supported way to close it is not an event but a read: at `agent_settled`, compare the repository with a snapshot taken at `before_agent_start` and let `tsp check --json --paths ... --since <head>` answer. The extension does this since #345 (`gitWatch`), and the live harness shows the offer for a `sed -i` and for a write by another process. `tsp check --json` without `--paths` already reports a `sed -i` change, and `--since <commit>` reports a unit already committed.

## Limits of this proof

- The model is scripted. It proves the seam between Pi's runtime and the extension, not how a real model behaves when the offer is accepted (it does not load the skill).
- The delegated-worker case is modelled as a write by another process; the real subagent runner was not started.
- RPC mode has `hasUI = true`; the fullscreen TUI rendering of the question and of status text was not observed.
- A harness that leaves stdin open makes `pi -p` wait for it until it is killed; the harness runs JSON mode with `stdin` ignored.
