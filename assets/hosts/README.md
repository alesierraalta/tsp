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
`pi/tsp.ts` registers native `/tsp` and backward-compatible `/tpp` commands (`check`, `feedback --summary`, `doctor`) through the same dispatcher, spawning the `tsp` CLI. When a run ends after the agent edited source code (an `edit` or `write` result on a non-test code file), it asks `tsp check --json --paths <each path>` — adding `--since <the commit the turn started from>` so a work unit that was committed before the turn ended is still answered for — and follows the verdict: `none` ends in silence, `required` sends "haz el TSP" as a user message without asking (the repository decided, and a notification says why), and `ask` shows the question "¿Quieres hacer TSP?" in the chat: "Sí" sends "haz el TSP", which invokes `test-strategy`; "No" is remembered for those files. It does not ask after a TSP run, an aborted run, without UI, in a delegated subagent session (`GENTLE_PI_AGENTS_CHILD=1`, which has nobody to answer), or in a repo with `.no-testing-gate`; changes made through a shell command are not seen. A TSP run is a run whose agent loads the test-strategy skill (its installed `SKILL.md` read through `read` or a `cat`-style `bash` command, or a `/skill:test-strategy` command), or whose prompt is exactly `haz el TSP`; a message that only mentions tsp, a command that only names the file, and the copy of the skill inside the working repository (outside its `.pi/` folder) do not count. When a TSP run ends (same exclusions), it runs `tsp findings record` then `tsp findings check` and shows the review as a notification: info when nothing is unresolved, warning for unresolved findings, error when the findings could not be judged or record could not run (check is then skipped). The agent never sees it. Then it runs `tsp feedback --template` and sends the agent that template, asking it to record its feedback on the run with `tsp feedback --file`; nothing is sent when the reply carries no `verdict:` field: silently when feedback is disabled, with a warning naming why for an empty reply or a failing tsp. Whether the agent submits the report is not checked, and the feedback run itself triggers no review, no TSP question and no further request. It adds no skills and none of Gentle's surfaces (ODD, Engram, review lifecycle remain Gentle's). `tsp sync --hosts pi` is the only installer for it and writes just one file,
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
| Pi | native `/tsp` command and `/tpp` compatibility alias via the extension (`sync --hosts pi` → `~/.pi/agent/extensions/tpp/index.ts`); Stop command hook, Claude-compatible payload | `/tsp` and `/tpp` dispatch and the TSP offer covered by `pi/tsp.test.mjs` (run in CI), the offer watched in a live Pi 1.3.0 session on 2026-10-17 (offer after an edit, "No" remembered per file, "Sí" runs test-strategy, no offer after the TSP run, after a shell-only edit, or with `.no-testing-gate`); Stop hook contract read from the pi-hooks package; not run here |
| OpenCode | plugin event, disabled (adapter export only) | contract read from the OpenCode plugin docs; not run here |
| Codex, Gemini CLI | not implemented | their payload and output schemas are not verified here |

"Not run here" means exactly that: the shape comes from each project's own documentation, and
nobody has watched it fire on this machine. A hook that looks wired and never answers is the
failure this project keeps finding, so treat every row carrying that phrase as ready to test, not as
working.
