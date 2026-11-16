#!/usr/bin/env node
// Live proof of the Pi TSP extension: the real Pi runtime, the real assets/hosts/pi/tsp.ts and a real tsp binary,
// with a scripted model (scripted-model.ts) so the run is deterministic, offline and free.
//   TSP_BIN=./bin/tsp node assets/hosts/pi/live/run.mjs [case-name-filter]
// Needs `pi` on PATH. Not part of `go test` or CI: it drives a real Pi. Exit 0 when every case behaves as declared.
//
// A case with `gap` documents behaviour the extension does not have yet. It passes while the gap is open and FAILS
// as "gap closed" the moment the wanted behaviour appears, so the marker is removed with the fix that closes it.
import { spawn, execFileSync } from "node:child_process";
import { chmodSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const HERE = dirname(fileURLToPath(import.meta.url));
const PI_DIR = resolve(HERE, "..");
const ROOT = resolve(HERE, "../../../..");
const TSP_BIN = resolve(process.env.TSP_BIN ?? join(ROOT, "bin", "tsp"));
const QUIET_MS = 2500; // no event for this long after a settle: the extension has nothing more to do
const OFFER = "¿Quieres hacer TSP?";
const TSP_PROMPT = "haz el TSP";

const edit = (path, n = 1) => ({ tool: "write", args: { path, content: `export const v = ${n}\n` } });
const text = (t = "ok") => ({ text: t });
const fillers = Array.from({ length: 8 }, () => text());

/** The text of a user message or of a custom message an extension added (a continuation), whatever shape Pi gives its content. */
const textOf = (m) => (typeof m.content === "string" ? m.content : String(m.content?.[0]?.text ?? ""));
const offers = (r) => r.ui.filter((u) => u.method === "select" && String(u.title).startsWith(OFFER));
const tspRuns = (r) => r.users.filter((u) => u === TSP_PROMPT).length;
const statuses = (r) => r.ui.filter((u) => u.method === "setStatus").map((u) => u.statusText);
const tspAsked = (r) => r.seen.filter((m) => m.text.startsWith(TSP_PROMPT)).length; // what reached the model: a user message or a continuation
const checks = (r) => r.tspCalls.filter((c) => c.startsWith("check")).length;

/** Each case: what it sets up, what the model does, and the behaviour wanted. */
const PLAN_WITHOUT_FINDINGS = [
  "# Test plan",
  "",
  "| Layer | Skill | Scope | Status | Run |",
  "|---|---|---|---|---|",
  "",
  "## Findings",
  "",
  "| Id | Finding (path:line, one line) | Severity | Data safe? | Evidence id | Pinning test (suite path :: test name) | Status | Verdict by / date | Reason | Cited-files fingerprint at verdict |",
  "|---|---|---|---|---|---|---|---|---|---|",
  "",
].join("\n");

const CASES = [
  { name: "offer fires for an edit to non-test source", script: [edit("src/add.js"), text()], prompts: ["change it"], answer: () => "No",
    want: (r) => offers(r).length === 1 },
  { name: "answering Sí runs TSP in the same session", script: [edit("src/add.js"), text(), ...fillers], prompts: ["change it"], answer: () => "Sí",
    want: (r) => tspRuns(r) === 1 },
  { name: "answering No is remembered for those files, not for others", script: [edit("src/add.js", 1), text(), edit("src/add.js", 2), text(), edit("src/other.js"), text()],
    prompts: ["change it", "change it again", "now another file"], answer: () => "No", want: (r) => offers(r).length === 2 },
  { name: "a No survives a resume of the session (--continue)", script: [edit("src/add.js", 1), text()], prompts: ["change it"], answer: () => "No",
    then: { script: [edit("src/add.js", 2), text()], prompts: ["change it again"] }, want: (r) => offers(r).length === 1 && r.settled === 2 },
  { name: "the status line shows what is owed once the offer is raised", script: [edit("src/add.js"), text()], prompts: ["change it"], answer: () => "No",
    // The line must show the debt while the offer is open; answering No clears it, so the check is that it showed.
    want: (r) => statuses(r).includes("tsp: ask") },
  { name: "/tsp run starts a TSP run from the command line of the session", dirty: { "src/add.js": "export const add = (a, b) => a + b // earlier work\n" },
    script: [text("tsp ran"), ...fillers], prompts: ["/tsp run"], answer: () => "No", want: (r) => tspRuns(r) === 1 && offers(r).length === 0 },
  { name: "no second offer for edits made during the TSP run itself", script: [edit("src/add.js"), text(), edit("src/other.js"), text(), ...fillers],
    prompts: ["change it"], answer: () => "Sí", want: (r) => offers(r).length === 1 && tspRuns(r) === 1 },
  { name: "no offer after an aborted run", script: [edit("src/add.js"), { hang: true }], prompts: ["change it"], abortAfterTool: true, answer: () => "No",
    want: (r) => offers(r).length === 0 },
  { name: "no offer with .no-testing-gate", files: { ".no-testing-gate": "" }, script: [edit("src/add.js"), text()], prompts: ["change it"], answer: () => "No",
    want: (r) => offers(r).length === 0 && checks(r) === 0 },
  { name: "no offer and no TSP request without a UI when the verdict is ask", mode: "json", script: [edit("src/add.js"), text()], prompts: ["change it"],
    want: (r) => offers(r).length === 0 && tspAsked(r) === 0 },
  { name: "a required verdict runs TSP without asking (UI session)", files: { ".tsp.json": '{"gate":"always"}\n' }, script: [edit("src/add.js"), text(), ...fillers],
    prompts: ["change it"], answer: () => "No", want: (r) => offers(r).length === 0 && tspRuns(r) === 1 },
  { name: "a Gentle session runs TSP without asking and without a declared gate (UI session)", gentle: true, script: [edit("src/add.js"), text(), ...fillers],
    prompts: ["change it"], answer: () => "No", want: (r) => offers(r).length === 0 && tspRuns(r) === 1 },
  { name: "a Gentle session without a UI asks the model to run TSP, once, without a declared gate", gentle: true, mode: "json",
    script: [edit("src/add.js"), text(), ...fillers], prompts: ["change it"], want: (r) => tspAsked(r) === 1 },
  { name: "offer fires for a source change made by sed -i through bash", script: [{ tool: "bash", args: { command: "sed -i 's/a + b/a + b + 0/' src/add.js" } }, text()],
    prompts: ["change it"], answer: () => "No", want: (r) => offers(r).length === 1 },
  { name: "offer fires for a source change made by another process (a delegated worker)",
    script: [{ tool: "bash", args: { command: "node -e \"require('fs').writeFileSync('src/add.js','export const add=(a,b)=>a+b+1\\n')\"" } }, text()],
    prompts: ["change it"], answer: () => "No", want: (r) => offers(r).length === 1 },
  // #472: the orchestrator sits in the main checkout while its writers work in linked worktrees. The change is
  // outside ctx.cwd, and a check rooted there refuses it as out of repository; the extension groups it by the
  // worktree that holds it and checks it there, so the offer fires for what the delegate left behind.
  { name: "offer fires for a source change left in a linked worktree outside the session cwd (#472)", worktree: true,
    script: [{ tool: "bash", args: { command: "node -e \"require('fs').writeFileSync('../wt/src/add.js','export const add=(a,b)=>a+b+1\\n')\"" } }, text()],
    prompts: ["delegate the change"], answer: () => "No", want: (r) => offers(r).length === 1 && r.tspCalls.some((c) => c.startsWith("check --cwd ") && c.includes("/wt --json")) },
  { name: "no offer for a file that was dirty before the turn and not touched by it", dirty: { "src/add.js": "export const add = (a, b) => a + b // earlier work\n" },
    script: [text("nothing to change")], prompts: ["explain it"], answer: () => "No", want: (r) => offers(r).length === 0 && checks(r) === 0 },
  { name: "a required verdict without a UI asks the model to run TSP, once", mode: "json", files: { ".tsp.json": '{"gate":"always"}\n' },
    script: [edit("src/add.js"), text(), ...fillers], prompts: ["change it"], want: (r) => tspAsked(r) === 1 },
  // A run that wrote a test and recorded no finding loses it: the plan is what every later reader sees (#398).
  { name: "a TSP run that pinned a test and recorded nothing is asked to record it",
    files: { "docs/testing/test-plan.md": PLAN_WITHOUT_FINDINGS },
    script: [edit("src/add.test.js"), text(), ...fillers], prompts: ["haz el TSP"], answer: () => "No",
    want: (r) => r.users.filter((u) => u.includes("tsp plan add-finding")).length === 1 },
  // #471: without a UI and with the automatic run on, the extension drives the loop itself. The fix continuation it
  // sends is a user message, which Pi defers out of `agent_settled` and runs as the next request of the same
  // print-mode session: one more model request and one more settled run, with nothing rendered. `tsp` is scripted for
  // the two answers the loop reads, so the case proves the headless continuation and not the repository's plan.
  { name: "a TSP loop without a UI sends the fix continuation and Pi runs it", mode: "json", headless: true,
    tsp: {
      "findings record": [{ out: "F1 recorded src/add.test.js TestAdd", exit: 0 }],
      "findings next": [{ out: JSON.stringify({ next: "fix", round: 1, max_rounds: 5, tree: "t1", items: [{ id: "F1" }], reason: "round 1 of 5 left 1 item unsettled", review: "review", message: "F1 open src/add.test.js TestAdd" }), exit: 1 }],
    },
    script: [edit("src/add.js"), text(), text("fixed it")], prompts: ["haz el TSP"],
    want: (r) => r.settled >= 2 && r.seen.some((m) => m.text.startsWith("The TSP loop found defects to settle.")) },
  // #471: the loop judges commits, so `findings record` refuses an uncommitted tree. A session without a UI has nobody
  // to read that, so the extension asks the agent to commit and the turn it asks for records the round and continues.
  { name: "a TSP loop without a UI asks the agent to commit, and records after it", mode: "json", headless: true,
    tsp: {
      "findings record": [
        { out: "tsp findings: the working tree has uncommitted changes: commit them, then record, so the loop judges a commit", exit: 2 },
        { out: "F1 recorded src/add.test.js TestAdd", exit: 0 },
      ],
      "findings next": [{ out: JSON.stringify({ next: "fix", round: 1, max_rounds: 5, tree: "t1", items: [{ id: "F1" }], reason: "round 1 of 5 left 1 item unsettled", review: "review", message: "F1 open src/add.test.js TestAdd" }), exit: 1 }],
    },
    // Named paths, as the commit request asks: the tests it names, never a blanket add.
    script: [edit("src/add.test.js"), text(), { tool: "bash", args: { command: "git add -- src/add.test.js && git commit -qm tests" } }, text(), text("fixed it")],
    prompts: ["haz el TSP"],
    want: (r) => r.commits > 0 &&
      r.users.some((u) => u.startsWith("The TSP loop cannot judge an uncommitted working tree.")) &&
      r.users.some((u) => u.startsWith("The TSP loop found defects to settle.")) },

];

/** The logging `tsp` wrapper the extension finds first on PATH: the real binary, except for the answers a case scripts. */
function tspWrapper(log, stub = {}) {
  const spec = JSON.stringify({ bin: TSP_BIN, log, stub });
  return `#!/usr/bin/env node
// The logging wrapper: the real tsp, except for the subcommands this case scripts (one answer per call, in order).
const { appendFileSync, existsSync, readFileSync, writeFileSync } = require("node:fs");
const { spawnSync } = require("node:child_process");
const spec = ${spec};
const argv = process.argv.slice(2);
const line = argv.join(" ");
appendFileSync(spec.log, line + "\\n");
const countersFile = spec.log + ".counters";
const counters = existsSync(countersFile) ? JSON.parse(readFileSync(countersFile, "utf8")) : {};
for (const [prefix, answers] of Object.entries(spec.stub)) {
  if (!line.startsWith(prefix)) continue;
  const i = counters[prefix] ?? 0;
  counters[prefix] = i + 1;
  writeFileSync(countersFile, JSON.stringify(counters));
  const answer = answers[Math.min(i, answers.length - 1)];
  process.stdout.write(answer.out + "\\n");
  process.exit(answer.exit);
}
process.exit(spawnSync(spec.bin, argv, { stdio: "inherit" }).status ?? 1);
`;
}

function sandbox(c) {
  const dir = mkdtempSync(join(tmpdir(), "pi-live-"));
  const repo = join(dir, "repo"), agent = join(dir, "agent"), bin = join(dir, "bin"), log = join(dir, "tsp-calls.log");
  for (const d of [repo, agent, bin, join(dir, "xdg"), join(repo, "src")]) mkdirSync(d, { recursive: true });
  writeFileSync(join(repo, "src/add.js"), "export const add = (a, b) => a + b\n");
  writeFileSync(join(repo, "package.json"), '{"name":"x","scripts":{"test":"echo ok"}}\n');
  for (const [name, body] of Object.entries(c.files ?? {})) { mkdirSync(dirname(join(repo, name)), { recursive: true }); writeFileSync(join(repo, name), body); }
  const git = (...a) => execFileSync("git", ["-C", repo, ...a], { stdio: "ignore" });
  git("init", "-q"); git("config", "user.email", "live@example.test"); git("config", "user.name", "live");
  git("add", "-A"); git("commit", "-qm", "init");
  // A linked worktree outside the checkout, the way branch isolation gives a writer its own tree (#472).
  const worktree = join(dir, "wt");
  if (c.worktree) git("worktree", "add", "-q", worktree, "-b", "wt");
  for (const [name, body] of Object.entries(c.dirty ?? {})) writeFileSync(join(repo, name), body); // left uncommitted: earlier work
  // A logging wrapper: the extension runs `tsp` from PATH, and this is how a silent extension is told from a broken
  // one. A case may script the answers of the subcommands the loop reads (`tsp`), so a loop continuation is observed
  // without a repository that has a plan to judge; everything else runs the real binary.
  writeFileSync(join(bin, "tsp"), tspWrapper(log, c.tsp));
  chmodSync(join(bin, "tsp"), 0o755);
  // The headless loop is opt-in, so a case that drives it turns it on where the extension's own `tsp status --json`
  // reads it: the sandbox's throwaway state root, never the operator's.
  if (c.headless) {
    const root = join(dir, "xdg", "tsp");
    mkdirSync(root, { recursive: true });
    writeFileSync(join(root, "state.json"), JSON.stringify({ schemaVersion: 1, features: { "headless-loop": { enabled: true } } }) + "\n");
  }
  const env = { ...process.env, PATH: `${bin}:${process.env.PATH}`, PI_CODING_AGENT_DIR: agent, XDG_CONFIG_HOME: join(dir, "xdg"),
    PI_OFFLINE: "1", PI_SKIP_VERSION_CHECK: "1", PI_LIVE_SCRIPT: JSON.stringify(c.script) };
  delete env.GENTLE_PI_AGENTS_CHILD;
  return { dir, repo, worktree, log, env };
}

const BASE_FLAGS = ["--no-extensions", ...(process.env.PI_LIVE_WITHOUT_TSP ? [] : ["--extension", process.env.PI_LIVE_EXTENSION ?? join(PI_DIR, "tsp.ts")]), "--extension", join(HERE, "scripted-model.ts"),
  "--provider", "scripted", "--model", "script", "--no-context-files", "--no-skills"];
/** A case with `then` runs a second Pi process on the same stored session (--continue): what survives a resume. */
// A case with `gentle` also loads a stand-in for gentle-pi, so the session is observed as a Gentle one (#418).
const piFlags = (sb, { persist = false, cont = false, gentle = false } = {}) =>
  [...BASE_FLAGS, ...(gentle ? ["--extension", join(HERE, "gentle-stub.ts")] : []),
    ...(persist ? ["--session-dir", join(sb.dir, "sessions"), ...(cont ? ["--continue"] : [])] : ["--no-session"])];
const scriptEnv = (sb, c) => ({ ...sb.env, PI_LIVE_SCRIPT: JSON.stringify(c.script), PI_LIVE_SEEN: join(sb.dir, "seen.jsonl") });

/** One RPC session: send each prompt, answer dialogs, and wait for the extension to go quiet after the run settles. */
function runRpc(c, sb, session) {
  return new Promise((done) => {
    const r = { ui: [], users: [], events: new Set(), settled: 0, aborted: [], tspCalls: [] };
    const p = spawn("pi", ["--mode", "rpc", ...piFlags(sb, { ...session, gentle: c.gentle })], { cwd: sb.repo, env: scriptEnv(sb, c) });
    let buf = "", last = Date.now(), queue = [...c.prompts], busy = false, asked = 0;
    const send = (o) => p.stdin.write(JSON.stringify(o) + "\n");
    p.stdout.on("data", (d) => {
      buf += d;
      for (let i; (i = buf.indexOf("\n")) >= 0; ) {
        const line = buf.slice(0, i); buf = buf.slice(i + 1);
        let o; try { o = JSON.parse(line); } catch { continue; }
        last = Date.now(); r.events.add(o.type);
        if (o.type === "extension_ui_request") {
          r.ui.push(o);
          if (["select", "confirm", "input", "editor"].includes(o.method)) send({ type: "extension_ui_response", id: o.id, value: c.answer(asked++, o) });
        } else if (o.type === "message_start" && (o.message?.role === "user" || o.message?.customType)) r.users.push(textOf(o.message));
        else if (o.type === "tool_execution_end" && c.abortAfterTool) send({ type: "abort" });
        else if (o.type === "agent_settled") { r.settled++; r.aborted.push(o.aborted); busy = false; }
      }
    });
    const timer = setInterval(() => {
      if (!busy && queue.length > 0 && Date.now() - last > 300) { busy = true; send({ type: "prompt", message: queue.shift() }); }
      else if (!busy && queue.length === 0 && r.settled > 0 && Date.now() - last > QUIET_MS) finish();
      else if (Date.now() - last > 30000) finish();
    }, 100);
    function finish() { clearInterval(timer); p.kill(); r.tspCalls = readCalls(sb.log); done(r); }
    p.on("error", () => finish());
  });
}

/** Single-shot JSON mode: no UI exists, so any offer or dialog would be a defect. */
function runJson(c, sb) {
  return new Promise((done) => {
    const r = { ui: [], users: [], events: new Set(), settled: 0, aborted: [], tspCalls: [] };
    const p = spawn("pi", ["-p", c.prompts[0], "--mode", "json", ...piFlags(sb, { gentle: c.gentle })], { cwd: sb.repo, env: scriptEnv(sb, c), stdio: ["ignore", "pipe", "pipe"] });
    let buf = "";
    p.stdout.on("data", (d) => {
      buf += d;
      for (let i; (i = buf.indexOf("\n")) >= 0; ) {
        const line = buf.slice(0, i); buf = buf.slice(i + 1);
        let o; try { o = JSON.parse(line); } catch { continue; }
        r.events.add(o.type);
        if (o.type === "message_start" && (o.message?.role === "user" || o.message?.customType)) r.users.push(textOf(o.message));
        if (o.type === "agent_settled") { r.settled++; r.aborted.push(o.aborted); }
      }
    });
    const timer = setTimeout(() => p.kill(), 30000);
    p.on("close", () => { clearTimeout(timer); r.tspCalls = readCalls(sb.log); done(r); });
    p.on("error", () => { clearTimeout(timer); done(r); });
  });
}

function readLines(path) {
  try { return readFileSync(path, "utf8").split("\n").filter(Boolean); } catch { return []; }
}
const readCalls = readLines;

const filter = process.argv[2];
const piVersion = execFileSync("pi", ["--version"], { encoding: "utf8" }).trim();
const commit = execFileSync("git", ["-C", ROOT, "rev-parse", "--short", "HEAD"], { encoding: "utf8" }).trim();
console.log(`pi ${piVersion} · extension commit ${commit} · tsp ${TSP_BIN}\n`);
let failed = 0;
const seenEvents = new Set(), seenUiKeys = new Set(), seenMethods = new Set();
for (const c of CASES.filter((x) => !filter || x.name.includes(filter))) {
  const sb = sandbox(c);
  const phases = [c, ...(c.then ? [{ ...c, ...c.then }] : [])];
  let r;
  for (const [i, phase] of phases.entries()) {
    const part = await (phase.mode === "json" ? runJson : runRpc)(phase, sb, { persist: phases.length > 1, cont: i > 0 });
    r = r ? { ...part, ui: [...r.ui, ...part.ui], users: [...r.users, ...part.users], events: new Set([...r.events, ...part.events]), settled: r.settled + part.settled, aborted: [...r.aborted, ...part.aborted] } : part;
  }
  // A case only counts when the session ran AND the repository really changed: a sed that matched nothing would fake a gap.
  r.seen = readLines(join(sb.dir, "seen.jsonl")).flatMap((l) => { try { return [JSON.parse(l)]; } catch { return []; } });
  r.changed = [execFileSync("git", ["-C", sb.repo, "status", "--porcelain"], { encoding: "utf8" }),
    ...(c.worktree ? [execFileSync("git", ["-C", sb.worktree, "status", "--porcelain"], { encoding: "utf8" })] : [])].join("\n");
  // A case may commit what it changed, so the repository really changed even with a clean tree (one commit above init).
  r.commits = Number(execFileSync("git", ["-C", sb.repo, "rev-list", "--count", "HEAD"], { encoding: "utf8" }).trim()) - 1;
  const ran = r.settled > 0 && (r.changed.includes("src/") || r.commits > 0);
  const wanted = ran && c.want(r);
  const verdict = !ran ? "FAIL (no run or no change)" : c.gap ? (wanted ? "GAP CLOSED, remove the marker" : `known gap ${c.gap}`) : wanted ? "pass" : "FAIL";
  if (verdict.startsWith("FAIL") || verdict.startsWith("GAP CLOSED")) failed++;
  console.log(`${verdict.padEnd(14)} ${c.name}`);
  console.log(`               statuses=${JSON.stringify(statuses(r))}`);
  console.log(`               offers=${offers(r).length} tsp-runs=${tspRuns(r)} check-calls=${checks(r)} settled=${r.settled} changed=${JSON.stringify(r.changed.split("\n").filter(Boolean).map((l) => l.slice(3)).join(","))} aborted=${JSON.stringify(r.aborted)} users=${JSON.stringify(r.users.map((u) => u.slice(0, 24)))}`);
  r.events.forEach((e) => seenEvents.add(e));
  r.ui.forEach((u) => { seenMethods.add(u.method); Object.keys(u).forEach((k) => seenUiKeys.add(k)); });
  rmSync(sb.dir, { recursive: true, force: true });
}
console.log(`\nevent types seen: ${[...seenEvents].sort().join(", ")}\nextension_ui_request methods: ${[...seenMethods].sort().join(", ")} · fields: ${[...seenUiKeys].sort().join(", ")}`);
console.log(failed === 0 ? "\nall cases behave as declared" : `\n${failed} case(s) differ from what is declared`);
process.exit(failed === 0 ? 0 : 1);
