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
    want: (r) => statuses(r).at(-1) === "tsp: ask" },
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
  { name: "offer fires for a source change made by sed -i through bash", script: [{ tool: "bash", args: { command: "sed -i 's/a + b/a + b + 0/' src/add.js" } }, text()],
    prompts: ["change it"], answer: () => "No", want: (r) => offers(r).length === 1 },
  { name: "offer fires for a source change made by another process (a delegated worker)",
    script: [{ tool: "bash", args: { command: "node -e \"require('fs').writeFileSync('src/add.js','export const add=(a,b)=>a+b+1\\n')\"" } }, text()],
    prompts: ["change it"], answer: () => "No", want: (r) => offers(r).length === 1 },
  { name: "no offer for a file that was dirty before the turn and not touched by it", dirty: { "src/add.js": "export const add = (a, b) => a + b // earlier work\n" },
    script: [text("nothing to change")], prompts: ["explain it"], answer: () => "No", want: (r) => offers(r).length === 0 && checks(r) === 0 },
  { name: "a required verdict without a UI asks the model to run TSP, once", mode: "json", files: { ".tsp.json": '{"gate":"always"}\n' },
    script: [edit("src/add.js"), text(), ...fillers], prompts: ["change it"], want: (r) => tspAsked(r) === 1 },
];

function sandbox(c) {
  const dir = mkdtempSync(join(tmpdir(), "pi-live-"));
  const repo = join(dir, "repo"), agent = join(dir, "agent"), bin = join(dir, "bin"), log = join(dir, "tsp-calls.log");
  for (const d of [repo, agent, bin, join(dir, "xdg"), join(repo, "src")]) mkdirSync(d, { recursive: true });
  writeFileSync(join(repo, "src/add.js"), "export const add = (a, b) => a + b\n");
  writeFileSync(join(repo, "package.json"), '{"name":"x","scripts":{"test":"echo ok"}}\n');
  for (const [name, body] of Object.entries(c.files ?? {})) writeFileSync(join(repo, name), body);
  const git = (...a) => execFileSync("git", ["-C", repo, ...a], { stdio: "ignore" });
  git("init", "-q"); git("config", "user.email", "live@example.test"); git("config", "user.name", "live");
  git("add", "-A"); git("commit", "-qm", "init");
  for (const [name, body] of Object.entries(c.dirty ?? {})) writeFileSync(join(repo, name), body); // left uncommitted: earlier work
  // A logging wrapper: the extension runs `tsp` from PATH, and this is how a silent extension is told from a broken one.
  writeFileSync(join(bin, "tsp"), `#!/bin/sh\nprintf '%s\\n' "$*" >> "${log}"\nexec "${TSP_BIN}" "$@"\n`);
  chmodSync(join(bin, "tsp"), 0o755);
  const env = { ...process.env, PATH: `${bin}:${process.env.PATH}`, PI_CODING_AGENT_DIR: agent, XDG_CONFIG_HOME: join(dir, "xdg"),
    PI_OFFLINE: "1", PI_SKIP_VERSION_CHECK: "1", PI_LIVE_SCRIPT: JSON.stringify(c.script) };
  delete env.GENTLE_PI_AGENTS_CHILD;
  return { dir, repo, log, env };
}

const BASE_FLAGS = ["--no-extensions", ...(process.env.PI_LIVE_WITHOUT_TSP ? [] : ["--extension", process.env.PI_LIVE_EXTENSION ?? join(PI_DIR, "tsp.ts")]), "--extension", join(HERE, "scripted-model.ts"),
  "--provider", "scripted", "--model", "script", "--no-context-files", "--no-skills"];
/** A case with `then` runs a second Pi process on the same stored session (--continue): what survives a resume. */
const piFlags = (sb, { persist = false, cont = false } = {}) =>
  [...BASE_FLAGS, ...(persist ? ["--session-dir", join(sb.dir, "sessions"), ...(cont ? ["--continue"] : [])] : ["--no-session"])];
const scriptEnv = (sb, c) => ({ ...sb.env, PI_LIVE_SCRIPT: JSON.stringify(c.script), PI_LIVE_SEEN: join(sb.dir, "seen.jsonl") });

/** One RPC session: send each prompt, answer dialogs, and wait for the extension to go quiet after the run settles. */
function runRpc(c, sb, session) {
  return new Promise((done) => {
    const r = { ui: [], users: [], events: new Set(), settled: 0, aborted: [], tspCalls: [] };
    const p = spawn("pi", ["--mode", "rpc", ...piFlags(sb, session)], { cwd: sb.repo, env: scriptEnv(sb, c) });
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
    const p = spawn("pi", ["-p", c.prompts[0], "--mode", "json", ...piFlags(sb)], { cwd: sb.repo, env: scriptEnv(sb, c), stdio: ["ignore", "pipe", "pipe"] });
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
  r.changed = execFileSync("git", ["-C", sb.repo, "status", "--porcelain"], { encoding: "utf8" });
  const ran = r.settled > 0 && r.changed.includes("src/");
  const wanted = ran && c.want(r);
  const verdict = !ran ? "FAIL (no run or no change)" : c.gap ? (wanted ? "GAP CLOSED, remove the marker" : `known gap ${c.gap}`) : wanted ? "pass" : "FAIL";
  if (verdict.startsWith("FAIL") || verdict.startsWith("GAP CLOSED")) failed++;
  console.log(`${verdict.padEnd(14)} ${c.name}`);
  console.log(`               offers=${offers(r).length} tsp-runs=${tspRuns(r)} check-calls=${checks(r)} settled=${r.settled} changed=${JSON.stringify(r.changed.split("\n").filter(Boolean).map((l) => l.slice(3)).join(","))} aborted=${JSON.stringify(r.aborted)} users=${JSON.stringify(r.users.map((u) => u.slice(0, 24)))}`);
  r.events.forEach((e) => seenEvents.add(e));
  r.ui.forEach((u) => { seenMethods.add(u.method); Object.keys(u).forEach((k) => seenUiKeys.add(k)); });
  rmSync(sb.dir, { recursive: true, force: true });
}
console.log(`\nevent types seen: ${[...seenEvents].sort().join(", ")}\nextension_ui_request methods: ${[...seenMethods].sort().join(", ")} · fields: ${[...seenUiKeys].sort().join(", ")}`);
console.log(failed === 0 ? "\nall cases behave as declared" : `\n${failed} case(s) differ from what is declared`);
process.exit(failed === 0 ? 0 : 1);
