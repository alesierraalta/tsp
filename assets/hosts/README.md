# Host adapters

TSP is the current product name. The legacy `/tpp` command and `tpp` extension component
remain intentional compatibility surfaces: the component ID is `tpp`, and the installed
path stays `~/.pi/agent/extensions/tpp/index.ts` to preserve existing installation state.

The decisions live in the binary. A host adapter is transport: it answers when the agent
finishes, hands the binary the working directory, and puts the answer somewhere a person or a
model will read it.

Three shapes cover every host seen so far.

`tsp sync` installs the embedded skills into every host configuration directory it finds: Claude
Code reads `~/.claude/skills`, OpenCode reads `~/.config/opencode/skills`, Gemini reads
`~/.gemini/skills`, Anti-Gravity reads `~/.gemini/antigravity/skills`, and Codex reads
`~/.codex/skills`. Anti-Gravity keeps its tree inside the Gemini config directory, so discovery
asks for the CLI's `settings.json` before treating `~/.gemini` as the Gemini CLI; an agy-only
machine is agy, not a CLI with no configuration. Discovery never offers Pi — the Pi extension
is the explicit `tsp sync --hosts pi` opt-in below. The Stop hook is wired only where its transport
is known—Claude Code's `~/.claude/settings.json`; OpenCode, Gemini, and Codex receive the skills, but
their transports are documented rather than wired by `sync`. `tsp doctor` prints one line per host
with the skills it holds and the transport it really has, so no host claims a transport it does not
have.

**Command hooks.** The host runs a command when the agent stops and reads JSON back. Claude Code
and Pi both work this way, and Pi's payload carries the same fields Claude's does
(`session_id`, `transcript_path`, `cwd`, `hook_event_name`, `stop_hook_active`), so
`tsp gate` serves both. Wire it with `tsp sync` on Claude Code, or by merging
`pi/settings.stop-hook.json` into Pi's settings — Pi's is contract-read, not run here (see Verified
below).

**Plugins.** The host loads code and gives it an event bus. OpenCode works this way, but its
automatic transport is disabled until session-scoped evidence is available: `opencode/tsp.ts`
no longer runs a check on `session.idle`, because a repo-wide check with no session binding
behind it fails closed. The file remains a harmless adapter export so an already-copied
plugin keeps loading, and on its own it shows no notices. If you copied the old plugin into
`.opencode/plugin/tpp.ts` or `~/.config/opencode/plugin/`, replace or remove that copy by
hand—`tsp sync` installs skills but does not manage copied plugin files. Manual use is
unaffected: run `tsp check` from a shell to see what the change still owes, plan gaps
included.

**Extensions.** The host loads an extension file and calls back into it. Pi works this way:
`pi/tsp.ts` registers native `/tsp` and backward-compatible `/tpp` commands (`run`, `check`, `feedback --summary`, `doctor`) through the same dispatcher, spawning the `tsp` CLI. When a run ends after the turn changed source code — an `edit` or `write` result on a non-test code file, a source file whose content a shell command changed, or a work unit the turn committed — it asks `tsp check --json --paths <each path>` — adding `--since <the commit the turn started from>` so a work unit that was committed before the turn ended is still answered for — and follows the verdict (a source change owes a test, and a tested change still owes one TSP run until `tsp findings record` has booked a round for the branch, so the question comes once per branch instead of never once the agent writes its own test): `none` ends in silence, `required` sends "haz el TSP" as a user message without asking (the repository decided, and a notification says why), and `ask` shows the question "¿Quieres hacer TSP?" in the chat: "Sí" sends "haz el TSP", which invokes `test-strategy`; "No" is remembered for those files. It does not ask after a TSP run (a branch that already had one is not asked again even when `tsp findings record` could not book it, the case #342 reports), an aborted run, without UI, in a delegated subagent session (`GENTLE_PI_AGENTS_CHILD=1`, which has nobody to answer), or in a repo with `.no-testing-gate`. The offer's decisions live on the session branch, not in memory: the paths declined, the last fix answer, the loop message waiting on a feedback run, the exact message the extension last sent, and the marker that this branch already had its TSP run are appended with `pi.appendEntry("tsp-state", …)` when they change and rebuilt from `ctx.sessionManager.getBranch()` in `session_start` and `session_tree`, so a reload, `/resume` or `--continue` keeps them, `/fork` starts from the parent's branch, a new session starts empty, and state an older version wrote or a broken write left behind is ignored without an error. A loop message still waiting when the session resumes is delivered once there, so the loop continues instead of stalling. In a session without a UI (`pi -p`, a JSON or RPC client) nobody can answer the question, so only a `required` verdict is enforced: at the actionable boundary the extension returns one continuation carrying "haz el TSP", once per turn, never in a delegated child, after the branch's run, or after an aborted turn, while `ask`, `none` and a stale check stay silent. The continuation is a session entry, not a user message: it starts one more model request in the same run and shows nothing in a chat that does not exist. A headless TSP run still marks the branch, so it is not requested twice. With a UI, a status line says what the branch owes (`tsp: ask`, `tsp: required`, or the loop it is in, `tsp: fix · round 2/3 · 2 open`) and clears when nothing is owed; a status write never fails a turn when `tsp` is missing or too old. `/tsp run` starts the same run as accepting the offer and, in a delegated child, says why it does not. A report longer than a toast can hold — a findings review or a command's output — is shown whole in a panel, and the toast says where it is. The turn's own changes are read twice: `git status` is sampled when the turn starts and again when it ends, and a source file whose content digest differs is the turn's (a file that was already dirty keeps its digest and is not blamed on it), while the commits HEAD gained during the turn are read from `git diff --name-only <start>..<end>`, because a committed work unit leaves no worktree change to see. Both are read from the turn's own directory, so a run inside a subdirectory reads its own paths and drops changes outside it (porcelain status is root-relative, while `--paths` is not). A change it cannot read from git — no repository — falls back to the edit tools, so a shell-only change is then not seen. A TSP run is a run whose agent loads the test-strategy skill (its installed `SKILL.md` read through `read` or a `cat`-style `bash` command, or a `/skill:test-strategy` command), or whose prompt is exactly `haz el TSP`; a message that only mentions tsp, a command that only names the file, and the copy of the skill inside the working repository (outside its `.pi/` folder) do not count. The loop's fix, round and feedback messages are recognized only as the exact text the extension sent, so a user's message that merely starts like one is the user's own. When a TSP run ends (same exclusions), it runs `tsp findings record` then `tsp findings next --json` and notifies the review with every finding's line, followed by the reason and notes for a fix or a round, or by the whole message for an end (done, stop, error), which sends the agent nothing more. A `fix` verdict sends the agent the settlement instruction; `tsp` starts a focused next round; `done`, `stop`, and `error` send no further run. Stop and error are warnings. A fix run asks `next` again without recording or offering TSP; when its answer is the same fix over the same tree (`tree` and `message` unchanged: the agent changed nothing), the extension warns that the loop needs the user's decision and sends nothing, so a fix run that settles nothing cannot loop forever. If `next` answers with its usage text — a binary that predates the subcommand — the extension falls back to the `findings check` notice; any other answer it cannot use (a non-zero exit with a diagnostic, or no output at all) is shown as an error naming what `next` answered, followed by the review, and the loop stops there, because a judgment that failed must not read as a loop that had nothing left to judge; a record that cannot run stops the review. After each TSP run it asks for feedback with `tsp feedback --template` and, when a template is available, sends the loop instruction only after that feedback run settles. Disabled feedback sends the loop instruction immediately; empty or failing feedback emits a warning. An abort or an unrelated user message cancels a waiting instruction. Whether the agent submits the feedback report is not checked. It adds no skills and none of Gentle's surfaces (ODD, Engram, review lifecycle remain Gentle's). `tsp sync --hosts pi` is the only installer for it and writes just one file,
`~/.pi/agent/extensions/tpp/index.ts` — the destination stays `tpp` to preserve recorded installation state, sync, and uninstall behavior; never skills, never hook settings. The Pi Stop hook is a
separate, unverified surface (its row below is contract-read, not run here).

A host with neither still gets everything except "what did THIS session do": `tsp check`
reads git, the tests, and the plan when one exists, and needs no host at all, which is also what makes it usable
from a Makefile, a pre-push script, or CI.

## Verified

| Host | Transport | Verified here |
|---|---|---|
| Claude Code | Stop command hook | yes: wired, run, exit code checked by `tsp doctor` |
| Anything with a shell | `tsp check` | yes |
| Pi | native `/tsp` command and `/tpp` compatibility alias via the extension (`sync --hosts pi` → `~/.pi/agent/extensions/tpp/index.ts`); Stop command hook, Claude-compatible payload | `/tsp` and `/tpp` dispatch and the TSP offer covered by `pi/tsp.test.mjs` (run in CI), the offer watched in a live Pi 1.3.0 session on 2026-10-17 (offer after an edit, "No" remembered per file, "Sí" runs test-strategy, no offer after the TSP run or with `.no-testing-gate`); the shell-command and committed-work-unit paths are covered by `pi/tsp.test.mjs` and were exercised against a real repository, not yet watched in a live Pi session; the branch-state persistence (reload, resume, fork, corrupt state) and the headless continuation are covered by `pi/tsp.test.mjs`, not yet watched in a live Pi session; Stop hook contract read from the pi-hooks package; not run here |
| OpenCode | plugin event, disabled (adapter export only) | contract read from the OpenCode plugin docs; not run here |
| Codex, Gemini CLI | not implemented | their payload and output schemas are not verified here |

"Not run here" means exactly that: the shape comes from each project's own documentation, and
nobody has watched it fire on this machine. A hook that looks wired and never answers is the
failure this project keeps finding, so treat every row carrying that phrase as ready to test, not as
working.
