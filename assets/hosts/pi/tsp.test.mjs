// Deterministic tests for the Pi extension's dispatch, in-session detection, observation, and UI.
// The registered-callback test mocks execFile; no real child process or ambient PATH is used.
// Run: node --test assets/hosts/pi/tsp.test.mjs
import assert from "node:assert/strict";
import { mock, test } from "node:test";
import childProcess from "node:child_process";
import { syncBuiltinESMExports } from "node:module";
import tppExtension, {
  FEEDBACK_LEAD,
  GENTLE_OBSERVATION_ENV,
  GENTLE_OBSERVATION_VALUE,
  observesGentleSession,
  parseTppCommand,
  buildArgv,
  childEnv,
  runTppCommand,
  isSourcePath,
} from "./tsp.ts";

// The literals internal/mode pins on the Go side: the shared protocol, spelled out on both shores.
const protocolEnv = "TPP_GENTLE_OBSERVATION";
const protocolValue = "pi-session-gentle-active";

test("observation protocol matches Go literals; registered commands invoke the shared CLI", async (t) => {
  assert.equal(GENTLE_OBSERVATION_ENV, protocolEnv);
  assert.equal(GENTLE_OBSERVATION_VALUE, protocolValue);
  const calls = [];
  const pi = {
    registerCommand: (name, options) => calls.push({ name, options }),
    getAllTools: () => [],
    on: () => {},
  };
  tppExtension(pi);
  assert.deepEqual(calls.map(({ name }) => name), ["tsp", "tpp"]);
  assert.match(calls[0].options.description, /tsp/);
  const executions = [];
  const execFileMock = mock.method(childProcess, "execFile", (file, args, options, callback) => {
    executions.push({ file, args, cwd: options.cwd });
    callback(null, "ok\\n", "");
    return {};
  });
  syncBuiltinESMExports();
  t.after(() => {
    execFileMock.mock.restore();
    syncBuiltinESMExports();
  });
  const invocations = [];
  for (const { name, options } of calls) {
    const rec = harness();
    const ctx = {
      cwd: "/w", hasUI: true,
      ui: { notify: (...args) => rec.deps.notify(...args), setStatus: (_key, text) => rec.deps.setStatus(text) },
    };
    assert.equal(typeof options.handler, "function");
    await options.handler("check", ctx);
    invocations.push({ name, notifications: rec.rec.notify, status: rec.rec.status });
  }
  assert.deepEqual(invocations.map(({ name }) => name), ["tsp", "tpp"]);
  assert.deepEqual(executions, [
    { file: "tsp", args: ["check", "--cwd", "/w"], cwd: "/w" },
    { file: "tsp", args: ["check", "--cwd", "/w"], cwd: "/w" },
  ]);
  for (const { notifications, status } of invocations) {
    assert.deepEqual(notifications, [{ message: "ok\\n", type: "info" }]);
    assert.deepEqual(status, ["tsp check: ok"]);
  }
});

test("dispatch and argv expose the shared CLI's check, feedback --summary, and doctor", () => {
  assert.deepEqual(parseTppCommand("check"), { kind: "check", rest: [] });
  assert.deepEqual(parseTppCommand("check --path plan/tpp.md"), { kind: "check", rest: ["--path", "plan/tpp.md"] });
  assert.deepEqual(parseTppCommand("feedback --summary"), { kind: "feedback-summary" });
  assert.deepEqual(parseTppCommand("doctor"), { kind: "doctor", mode: "auto" });
  assert.deepEqual(parseTppCommand("doctor --mode gentle"), { kind: "doctor", mode: "gentle" });
  assert.deepEqual(parseTppCommand("doctor --mode auto"), { kind: "doctor", mode: "auto" });
  assert.deepEqual(buildArgv(parseTppCommand("check --path plan/tpp.md"), "/w"), ["check", "--cwd", "/w", "--path", "plan/tpp.md"]);
  assert.deepEqual(buildArgv(parseTppCommand("feedback --summary"), "/w"), ["feedback", "--summary"]);
  assert.deepEqual(buildArgv(parseTppCommand("doctor"), "/w"), ["doctor", "--mode", "auto"]);
  assert.deepEqual(buildArgv(parseTppCommand("doctor --mode gentle"), "/w"), ["doctor", "--mode", "gentle"]);
});

test("dispatch refuses anything outside that surface", () => {
  for (const raw of ["", "   ", "review", "feedback", "feedback --file x", "doctor --mode turbo", "doctor --json", "sync"]) {
    const d = parseTppCommand(raw);
    assert.equal(d.kind, "help", `${JSON.stringify(raw)} must route to help, got ${JSON.stringify(d)}`);
    assert.ok(d.reason.length > 0, "help names the reason");
  }
});

test("gentle detection reads registered tools, not PATH", () => {
  const cases = [
    [{ name: "gentle_review" }, true],
    [{ name: "gentle_review_scope" }, true],
    [{ name: "gentle_custom_thing" }, false, "exact Gentle-owned tool names, not a generic gentle_ prefix"],
    [{ name: "review", sourceInfo: { path: "/x/gentle-pi/extensions/gentle-ai.ts" } }, true],
    [{ name: "review", sourceInfo: { path: "/home/u/gentle-pi-fork/index.ts" } }, false, "whole path segments, never substrings"],
    [{ name: "review", sourceInfo: { path: "/x/some-ext/index.ts" } }, false],
    [{ name: "bash" }, false],
  ];
  for (const [tool, want, why] of cases) assert.equal(observesGentleSession([tool]), want, why ?? JSON.stringify(tool));
  assert.equal(observesGentleSession([]), false);
});

test("the observation rides only the doctor child, only when observed, and mutates nothing", () => {
  const base = Object.freeze({ PATH: "/usr/bin", HOME: "/home/u" });
  // The parent shell may legitimately export the protocol var; capture it so the
  // mutation check below compares before/after instead of assuming it is absent.
  const ambientBefore = process.env[protocolEnv];
  assert.equal(childEnv(base, parseTppCommand("doctor"), true)[protocolEnv], protocolValue);
  assert.equal(childEnv(base, parseTppCommand("doctor"), true).PATH, "/usr/bin", "the child keeps its normal environment");
  assert.equal(childEnv(base, parseTppCommand("check"), true)[protocolEnv], undefined, "check never needs a mode observation, whatever the shell exports");
  assert.equal(childEnv(base, parseTppCommand("doctor"), false)[protocolEnv], undefined, "detection failing never forges the observation");
  assert.deepEqual({ ...base }, { PATH: "/usr/bin", HOME: "/home/u" }, "the base env object is untouched");
  assert.equal(process.env[protocolEnv], ambientBefore, "childEnv never mutates this process's ambient environment");
  // The child's env comes from the caller's base, never from ambient process.env:
  // a base-borne value rides verbatim, even for check, where the extension adds nothing.
  const marker = "from-caller-base";
  const carry = childEnv({ ...base, [protocolEnv]: marker }, parseTppCommand("check"), true);
  assert.equal(carry[protocolEnv], marker, "childEnv copies the caller's base verbatim");
});

// A harness recording what the extension would do, without spawning anything.
function harness({ tools = [], hasUI = true, outcome = { stdout: "ok\n", stderr: "", code: 0 } } = {}) {
  const rec = { exec: [], notify: [], status: [] };
  const deps = {
    tools, cwd: "/w", env: { PATH: "/usr/bin" }, hasUI,
    exec: async (argv, opts) => { rec.exec.push({ argv, opts }); return outcome; },
    notify: (message, type) => rec.notify.push({ message, type }),
    setStatus: (text) => rec.status.push(text),
  };
  return { deps, rec };
}

test("/tpp check and feedback run the shared CLI in the session cwd with guarded UI", async () => {
  const check = harness();
  await runTppCommand("check", check.deps);
  assert.deepEqual(check.rec.exec[0].argv, ["check", "--cwd", "/w"]);
  assert.equal(check.rec.exec[0].opts.cwd, "/w");
  assert.equal(check.rec.notify[0].type, "info");
  assert.match(check.rec.notify[0].message, /ok/);
  assert.equal(check.rec.status.length, 1);
  const feedback = harness();
  await runTppCommand("feedback --summary", feedback.deps);
  assert.deepEqual(feedback.rec.exec[0].argv, ["feedback", "--summary"]);
});

test("/tpp doctor passes the observation only under a live gentle session", async () => {
  const gentle = harness({ tools: [{ name: "gentle_review" }] });
  await runTppCommand("doctor", gentle.deps);
  assert.equal(gentle.rec.exec[0].opts.env[protocolEnv], protocolValue);
  const plain = harness({ tools: [{ name: "bash" }] });
  await runTppCommand("doctor", plain.deps);
  assert.equal(plain.rec.exec[0].opts.env[protocolEnv], undefined);
  assert.deepEqual(plain.rec.exec[0].argv, ["doctor", "--mode", "auto"]);
});

test("failures surface through notify and status instead of throwing at Pi", async () => {
  const failed = harness({ outcome: { stdout: "", stderr: "tpp: pending\n", code: 1 } });
  await runTppCommand("check", failed.deps);
  assert.equal(failed.rec.notify[0].type, "error");
  assert.match(failed.rec.notify[0].message, /pending/);
  assert.match(failed.rec.status[0], /exit 1/);
  const missing = harness();
  missing.deps.exec = async () => { const err = new Error("spawn tpp ENOENT"); err.code = "ENOENT"; throw err; };
  await runTppCommand("check", missing.deps);
  assert.equal(missing.rec.notify[0].type, "error");
  assert.match(missing.rec.notify[0].message, /tpp/);
});

test("without UI nothing renders — and help never spawns a process", async () => {
  const quiet = harness({ hasUI: false });
  await runTppCommand("check", quiet.deps);
  assert.equal(quiet.rec.exec.length, 1, "the command itself still runs headless");
  assert.deepEqual(quiet.rec.notify, [], "no notify without UI");
  assert.deepEqual(quiet.rec.status, [], "setStatus is guarded by hasUI");
  const help = harness();
  await runTppCommand("review", help.deps);
  assert.deepEqual(help.rec.exec, [], "an unknown subcommand never reaches a child process");
  assert.equal(help.rec.notify.length, 1);
  assert.match(help.rec.notify[0].message, /Usage/);
});

// The TSP offer: after a run that changed source code, Pi asks in the chat whether to run TSP; "Sí" sends the
// identity prompt "haz el TSP" as if the user typed it. Every collaborator is faked: no Pi, no files.
function offerHarness(opts = {}) {
  const { hasUI = true, optedOut = false, env = {}, findings = {}, check = { stdout: "run m1: layers swept: 1 of 3\n", stderr: "", code: 1 } } = opts;
  const answer = "answer" in opts ? opts.answer : "Sí"; // undefined means the user dismissed the question
  const asked = [], sent = [], calls = [], notes = [];
  // findings maps a findings subcommand to the child it answers with; a missing one answers exit 0, no output.
  // check is the pre-offer `tsp check` the offer derives its criterion from; the default reports one run still to plan.
  const exec = async (argv, o) => {
    calls.push({ argv: [...argv], cwd: o.cwd });
    const outcome = argv[0] === "check" && argv[1] === "--cwd" ? check : findings[argv[1]] ?? { stdout: "", stderr: "", code: 0 };
    if (outcome instanceof Error) throw outcome;
    return outcome;
  };
  const handlers = {};
  const pi = {
    registerCommand: () => {},
    getAllTools: () => [],
    on: (event, handler) => { handlers[event] = handler; },
    sendUserMessage: (text) => sent.push(text),
  };
  tppExtension(pi, { optedOut: () => optedOut, env, exec });
  const ctx = { hasUI, cwd: "/repo", ui: {
    select: async (title, options) => { asked.push({ title, options }); return answer; },
    notify: (message, type) => notes.push({ message, type }),
  } };
  const run = async (prompt, edits, { aborted = false } = {}) => {
    await handlers.before_agent_start?.({ type: "before_agent_start", messageType: "user", prompt }, ctx);
    for (const [toolName, path, isError = false] of edits) {
      await handlers.tool_result({ type: "tool_result", toolName, input: { path }, isError, content: [] }, ctx);
    }
    await handlers.agent_settled({ type: "agent_settled", aborted }, ctx);
  };
  return { asked, sent, calls, notes, run };
}

test("a run that edits source code ends with the TSP question, and Sí sends haz el TSP", async () => {
  const h = offerHarness();
  await h.run("add a retry limit", [["edit", "internal/retry/retry.go"], ["write", "src/new.ts"]]);
  assert.deepEqual(h.calls, [{ argv: ["check", "--cwd", "/repo", "--paths", "internal/retry/retry.go", "--paths", "src/new.ts"], cwd: "/repo" }], "the check is scoped to the source this run touched, one --paths per path, never to the whole repository");
  assert.deepEqual(h.asked, [{ title: "¿Quieres hacer TSP?\nrun m1: layers swept: 1 of 3", options: ["Sí", "No"] }]);
  assert.deepEqual(h.sent, ["haz el TSP"]);
});

test("a changed path carrying a comma is asked for as its own path, never folded into a list", async () => {
  const h = offerHarness();
  await h.run("rename the record type", [["edit", "internal/a,b/a,b.go"], ["edit", "src/new.ts"]]);
  assert.deepEqual(h.calls, [{ argv: ["check", "--cwd", "/repo", "--paths", "internal/a,b/a,b.go", "--paths", "src/new.ts"], cwd: "/repo" }], "the check splits its list on commas, so a path containing one has to travel in its own --paths");
  assert.equal(h.asked.length, 1, "a comma in a filename must not hide the debt");
});

test("the offer names the debt under the check's intro lines: a warning and the changed-file line never become the question", async () => {
  const h = offerHarness({ check: {
    stdout: "warning: git ignores plan/tpp.md: the run's evidence cannot be versioned\n1 production source file(s) changed: internal/retry/retry.go\nrun m1: layers swept: 1 of 3, ranked targets 0 of 2\n",
    stderr: "",
    code: 1,
  } });
  await h.run("add a retry limit", [["edit", "internal/retry/retry.go"]]);
  assert.deepEqual(h.asked, [{ title: "¿Quieres hacer TSP?\nrun m1: layers swept: 1 of 3, ranked targets 0 of 2", options: ["Sí", "No"] }]);
});

test("the offer asks for this change's own debt, so a plan target still open never becomes the question", async () => {
  // Scoped to the run's source, the check answers for that change alone: what reaches the question is its
  // debt. A plan's remaining targets are the run's agenda, and asking for them was the false alarm this pins.
  const h = offerHarness({ check: {
    stdout: "1 production source file(s) changed: internal/findings/findings.go\nno test was added or changed with it\n",
    stderr: "",
    code: 1,
  } });
  await h.run("fix the findings noise", [["edit", "internal/findings/findings.go"]]);
  assert.deepEqual(h.calls, [{ argv: ["check", "--cwd", "/repo", "--paths", "internal/findings/findings.go"], cwd: "/repo" }], "the check is scoped to this run's source");
  assert.deepEqual(h.asked, [{ title: "¿Quieres hacer TSP?\nno test was added or changed with it", options: ["Sí", "No"] }]);
});

test("only the plan data marker is parenthesized bookkeeping: any other parenthesized line is the debt", async () => {
  // `/^\(.*\)$/` once suppressed every parenthesized line; the only bookkeeping line of that
  // shape is the plan data marker, so a different parenthesized line is what the offer must show.
  const h = offerHarness({ check: {
    stdout: "1 production source file(s) changed: m1.go\n(plan m1 still owes 2 targets)\n",
    stderr: "",
    code: 1,
  } });
  await h.run("change a", [["edit", "m1.go"]]);
  assert.deepEqual(h.asked, [{ title: "¿Quieres hacer TSP?\n(plan m1 still owes 2 targets)", options: ["Sí", "No"] }]);
});

test("the reason reaching the UI is bounded: control characters stripped, cut at 300 with an ellipsis", async () => {
  // The reason can be plan-file text quoted verbatim into a UI question, so it is bounded
  // before it gets there: no control characters survive, and a long line is cut short.
  const long = `still pending: ${"x".repeat(280)}\u001b[31mred\u001b[0m`;
  const h = offerHarness({ check: { stdout: `1 production source file(s) changed: a.go\n${long}\n`, stderr: "", code: 1 } });
  await h.run("change a", [["edit", "a.go"]]);
  const title = h.asked[0]?.title ?? "";
  const reason = title.slice("¿Quieres hacer TSP?\n".length);
  assert.match(reason, /^still pending/, "the reason is still the pending line the check printed");
  assert.doesNotMatch(reason, /[\u0000-\u001f\u007f]/, "no control character ever reaches the UI question");
  assert.ok(reason.length <= 300, `the reason is capped at 300 characters, got ${reason.length}`);
  assert.ok(reason.endsWith("…"), "a cut reason ends with an ellipsis");
});

test("a cut never splits a character: the reason reaching the UI stays well-formed text", async () => {
  // The cut lands inside a character outside the Basic Multilingual Plane, which JavaScript stores as two halves.
  const long = `still pending: ${"x".repeat(283)}${"😀".repeat(50)}`;
  const h = offerHarness({ check: { stdout: `${long}\n`, stderr: "", code: 1 } });
  await h.run("change a", [["edit", "a.go"]]);
  const reason = (h.asked[0]?.title ?? "").slice("¿Quieres hacer TSP?\n".length);
  assert.ok(reason.endsWith("…"), "the long reason was cut");
  assert.ok(reason.isWellFormed(), "no half of a character reaches the UI question");
});

test("a reason made only of zero-width characters never reaches the UI blank", async () => {
  // U+200B is not a control character, so stripping controls leaves a line that renders as nothing.
  const owed = "no test was added or changed with it";
  const h = offerHarness({ check: { stdout: `\u200b\u200b\u200b\n${owed}\n`, stderr: "", code: 1 } });
  await h.run("change a", [["edit", "a.go"]]);
  assert.deepEqual(h.asked.map((a) => a.title), [`¿Quieres hacer TSP?\n${owed}`], "the offer names the debt, not a line nobody can read");
});

test("the cut cannot hide the only thing that painted: a padded line is not a reason", async () => {
  // The line passes the visibility test on its last character, and the cut then throws that character away.
  const owed = "no test was added or changed with it";
  const h = offerHarness({ check: { stdout: `${"\u200b".repeat(400)}a\n${owed}\n`, stderr: "", code: 1 } });
  await h.run("change a", [["edit", "a.go"]]);
  assert.deepEqual(h.asked.map((a) => a.title), [`¿Quieres hacer TSP?\n${owed}`], "what is shown has to be what was judged visible");
});

test("a reason of only combining marks is not visible text either", async () => {
  // Combining marks attach to a base character; alone they paint nothing, like the zero-width family.
  const owed = "no test was added or changed with it";
  const h = offerHarness({ check: { stdout: `\u0301\u0301\u0301\n${owed}\n`, stderr: "", code: 1 } });
  await h.run("change a", [["edit", "a.go"]]);
  assert.deepEqual(h.asked.map((a) => a.title), [`¿Quieres hacer TSP?\n${owed}`], "a line that paints nothing is not a reason");
});

test("bounded text is not visible text: a long invisible line never becomes the reason", async () => {
  // The cut runs before the visibility test, so a line longer than the cap gains a visible ellipsis it never had.
  const owed = "no test was added or changed with it";
  const h = offerHarness({ check: { stdout: `${"\u200b".repeat(400)}\n${owed}\n`, stderr: "", code: 1 } });
  await h.run("change a", [["edit", "a.go"]]);
  assert.deepEqual(h.asked.map((a) => a.title), [`¿Quieres hacer TSP?\n${owed}`], "an ellipsis the cut added is not a reason");
});

test("no C1 control character reaches the UI question either", async () => {
  // U+009B is the 8-bit control sequence introducer: a terminal treats it like ESC [.
  const h = offerHarness({ check: { stdout: "still pending: a\u009b31mred\n", stderr: "", code: 1 } });
  await h.run("change a", [["edit", "a.go"]]);
  const reason = (h.asked[0]?.title ?? "").slice("¿Quieres hacer TSP?\n".length);
  assert.match(reason, /^still pending: a/, "the pending line is still the reason");
  assert.doesNotMatch(reason, /[\u0080-\u009f]/, "no C1 control character ever reaches the UI question");
});

test("a line that is only control characters never hides the debt line after it", async () => {
  const owed = "no test was added or changed with it";
  const h = offerHarness({ check: { stdout: `\u001b\u001b\n${owed}\n`, stderr: "", code: 1 } });
  await h.run("change a", [["edit", "a.go"]]);
  assert.deepEqual(h.asked.map((a) => a.title), [`¿Quieres hacer TSP?\n${owed}`], "the check printed a debt, so the offer names it");
});

test("a check that exits 1 with no line at all asks nothing: an empty reply is no criterion", async () => {
  const h = offerHarness({ check: { stdout: "", stderr: "", code: 1 } });
  await h.run("change a", [["edit", "a.go"]]);
  assert.deepEqual(h.calls, [{ argv: ["check", "--cwd", "/repo", "--paths", "a.go"], cwd: "/repo" }]);
  assert.deepEqual(h.asked, [], "a check that says nothing offers nothing, never a sentence of its own");
  assert.deepEqual(h.sent, []);
  const meta = offerHarness({ check: { stdout: "1 production source file(s) changed: a.go\n", stderr: "", code: 1 } });
  await meta.run("change a", [["edit", "a.go"]]);
  assert.deepEqual(meta.asked, [], "bookkeeping alone names no debt");
});

test("a check that fails on its own terms asks nothing: its error text never becomes the question", async () => {
  const h = offerHarness({ check: { stdout: "", stderr: "tsp check: --path must name a plan file\n", code: 2 } });
  await h.run("change a", [["edit", "a.go"]]);
  assert.deepEqual(h.calls, [{ argv: ["check", "--cwd", "/repo", "--paths", "a.go"], cwd: "/repo" }]);
  assert.deepEqual(h.asked, [], "only exit 1 owes work; anything else means the check could not answer");
  assert.deepEqual(h.sent, []);
});

test("the offer speaks the debt line the check really prints, whatever its wording", async () => {
  // The literal exit-1 text `tsp check` emits for a change with no test beside it: the offer must never lose it.
  const owed = 'no test was added or changed with it: run the testing discipline ("haz el TSP"), record tsp check --no-test-needed "<reason>" when this change warrants no test, or add .no-testing-gate when this repository does not warrant it';
  const h = offerHarness({ check: { stdout: `1 production source file(s) changed: sample.go\n${owed}\n`, stderr: "", code: 1 } });
  await h.run("change a", [["edit", "sample.go"]]);
  assert.deepEqual(h.asked, [{ title: `¿Quieres hacer TSP?\n${owed}`, options: ["Sí", "No"] }]);
});

test("the offer follows the check: a change the repository does not owe a test for ends without a question", async () => {
  const h = offerHarness({ check: { stdout: "no production source is changed: nothing to check\n", stderr: "", code: 0 } });
  await h.run("change a", [["edit", "a.go"]]);
  assert.deepEqual(h.calls, [{ argv: ["check", "--cwd", "/repo", "--paths", "a.go"], cwd: "/repo" }]);
  assert.deepEqual(h.asked, [], "nothing owed is not an offer");
  assert.deepEqual(h.sent, []);
});

test("a check that cannot run asks nothing: no criterion, no question", async () => {
  const h = offerHarness({ check: new Error("spawn tsp ENOENT") });
  await h.run("change a", [["edit", "a.go"]]);
  assert.deepEqual(h.asked, []);
  assert.deepEqual(h.sent, []);
});

test("No sends nothing and the same files are not offered again; a new file is", async () => {
  const h = offerHarness({ answer: "No" });
  await h.run("change a", [["edit", "a.go"]]);
  await h.run("change a again", [["edit", "a.go"]]);
  assert.equal(h.asked.length, 1);
  await h.run("change b", [["edit", "b.go"]]);
  assert.equal(h.asked.length, 2);
  assert.deepEqual(h.sent, []);
});

test("no question when the run touched no source: tests, docs, config, failed edits, or nothing", async () => {
  const h = offerHarness();
  await h.run("tests and docs", [["edit", "a_test.go"], ["write", "src/a.test.ts"], ["edit", "tests/x.py"], ["edit", "README.md"], ["write", "config.json"], ["edit", "a.go", true]]);
  await h.run("just reading", []);
  assert.deepEqual(h.asked, []);
});

test("no question after a TSP run, an aborted run, without UI, or when the repo opted out", async () => {
  const tsp = offerHarness();
  await tsp.run("haz el TSP", [["edit", "a.go"]]);
  assert.deepEqual(tsp.asked, []);
  const aborted = offerHarness();
  await aborted.run("change a", [["edit", "a.go"]], { aborted: true });
  assert.deepEqual(aborted.asked, []);
  const headless = offerHarness({ hasUI: false });
  await headless.run("change a", [["edit", "a.go"]]);
  assert.deepEqual(headless.asked, []);
  const optedOut = offerHarness({ optedOut: true });
  await optedOut.run("change a", [["edit", "a.go"]]);
  assert.deepEqual(optedOut.asked, []);
});

test("a delegated child session never asks the TSP question nor touches the findings registry", async () => {
  const child = offerHarness({ env: { GENTLE_PI_AGENTS_CHILD: "1" } });
  await child.run("change a", [["edit", "a.go"]]);
  await child.run("haz el TSP", [["edit", "a_test.go"]]);
  assert.deepEqual(child.asked, []);
  assert.deepEqual(child.calls, []);
  assert.deepEqual(child.sent, []);
});

test("a TSP run ends by recording its witnesses, then shows the review of every finding", async () => {
  const review = "review 2 on feat/x: 1 fixed, 0 open, 1 new, 0 regressed\nF1 fixed a_test.go TestA\nF2 open b_test.go TestB\n";
  for (const [code, type] of [[0, "info"], [1, "warning"], [2, "error"]]) {
    const h = offerHarness({ findings: { record: { stdout: "F2 recorded b_test.go TestB\n", stderr: "", code: 0 }, check: { stdout: review, stderr: "", code } } });
    await h.run("haz el TSP", [["edit", "b_test.go"]]);
    assert.deepEqual(h.calls, [
      { argv: ["findings", "record", "--cwd", "/repo"], cwd: "/repo" },
      { argv: ["findings", "check", "--cwd", "/repo"], cwd: "/repo" },
      { argv: ["feedback", "--template"], cwd: "/repo" },
    ]);
    assert.deepEqual(h.notes, [{ message: review.trim(), type }], `check exit ${code}`);
    assert.deepEqual(h.asked, []);
  }
});

test("a TSP run that pinned nothing still shows the review; a record that cannot run stops before check", async () => {
  const none = offerHarness({ findings: { record: { stdout: "no new witness\n", stderr: "", code: 1 }, check: { stdout: "no finding recorded on feat/x\n", stderr: "", code: 0 } } });
  await none.run("haz el TSP", []);
  assert.deepEqual(none.calls.map((c) => c.argv[1]), ["record", "check", "--template"]);
  assert.deepEqual(none.notes, [{ message: "no finding recorded on feat/x", type: "info" }]);
  const detached = offerHarness({ findings: { record: { stdout: "", stderr: "tsp findings: detached HEAD\n", code: 2 } } });
  await detached.run("haz el TSP", []);
  assert.deepEqual(detached.calls.map((c) => c.argv[1]), ["record", "--template"], "a registry that cannot record still leaves the run's feedback to ask for");
  assert.deepEqual(detached.notes, [{ message: "tsp findings: detached HEAD", type: "error" }]);
  const missing = offerHarness({ findings: { record: new Error("spawn tsp ENOENT") } });
  await missing.run("haz el TSP", []);
  assert.deepEqual(missing.notes, [{ message: "tsp: spawn tsp ENOENT", type: "error" }]);
});

test("the findings review runs only after a TSP run the user asked for, with UI, not aborted, not opted out", async () => {
  for (const [h, prompt, aborted, offered] of [
    [offerHarness({ answer: "No" }), "change a", false, true],
    [offerHarness(), "haz el TSP", true, false],
    [offerHarness({ hasUI: false }), "haz el TSP", false, false],
    [offerHarness({ optedOut: true }), "haz el TSP", false, false],
  ]) {
    await h.run(prompt, [["edit", "a.go"]], { aborted });
    const why = `${prompt} aborted=${aborted}`;
    // The gate is proved by what it decided, not by a review call that never happens either way.
    assert.equal(h.asked.length, offered ? 1 : 0, `${why}: the offer decides before any review`);
    assert.deepEqual(h.calls.filter((c) => c.argv[0] === "findings"), [], why);
    assert.deepEqual(h.notes, [], why);
  }
});

const TEMPLATE = "# tsp run feedback\nts: 2026-10-22T20:10:29Z\nskill: test-strategy 0.7.5\npaid: \ncost: \nreason: \nverdict: \n";

test("a TSP run ends by asking the agent to record its feedback on the template tsp prints", async () => {
  const h = offerHarness({ findings: { "--template": { stdout: TEMPLATE, stderr: "", code: 0 } } });
  await h.run("haz el TSP", [["edit", "a_test.go"]]);
  assert.equal(h.sent.length, 1, "one feedback request per TSP run");
  const [prompt] = h.sent;
  assert.ok(prompt.startsWith(FEEDBACK_LEAD), prompt);
  assert.ok(prompt.endsWith(TEMPLATE.trim()), "the template reaches the agent whole, with the run's identity tsp filled in");
  assert.match(prompt, /tsp feedback --file/, "the agent is told how to submit it");
  assert.deepEqual(h.asked, [], "recording feedback asks the user nothing");
});

test("no feedback request when feedback is disabled, the template is empty, or tsp cannot run", async () => {
  for (const template of [
    { stdout: "feedback is disabled, so there is nothing to record: skip the retrospective. Enable it with: tsp feature enable feedback\n", stderr: "", code: 0 },
    { stdout: "", stderr: "", code: 0 },
    { stdout: "", stderr: "tsp: unknown flag\n", code: 2 },
    new Error("spawn tsp ENOENT"),
  ]) {
    const h = offerHarness({ findings: { "--template": template } });
    await h.run("haz el TSP", []);
    assert.deepEqual(h.sent, [], String(template.stdout ?? template.message));
  }
});

test("the feedback run itself reviews no findings, asks no TSP question and requests no more feedback", async () => {
  const h = offerHarness({ findings: { "--template": { stdout: TEMPLATE, stderr: "", code: 0 } } });
  await h.run("haz el TSP", []);
  const [prompt] = h.sent;
  const before = h.calls.length;
  await h.run(prompt, [["edit", "internal/a.go"]]);
  assert.equal(h.calls.length, before, "no findings review, no check, no template: the loop ends here");
  assert.deepEqual(h.asked, []);
  assert.equal(h.sent.length, 1);
  await h.run("change a", [["edit", "a.go"]]);
  assert.equal(h.asked.length, 1, "the next ordinary run is offered TSP again");
});

test("only a TSP run asks for feedback: an ordinary, aborted, headless, opted-out or child run never does", async () => {
  const template = { findings: { "--template": { stdout: TEMPLATE, stderr: "", code: 0 } } };
  for (const [h, prompt, aborted] of [
    [offerHarness({ ...template, answer: "No" }), "change a", false],
    [offerHarness(template), "haz el TSP", true],
    [offerHarness({ ...template, hasUI: false }), "haz el TSP", false],
    [offerHarness({ ...template, optedOut: true }), "haz el TSP", false],
    [offerHarness({ ...template, env: { GENTLE_PI_AGENTS_CHILD: "1" } }), "haz el TSP", false],
  ]) {
    await h.run(prompt, [["edit", "a.go"]], { aborted });
    assert.deepEqual(h.sent, [], `${prompt} aborted=${aborted}`);
    assert.deepEqual(h.calls.filter((c) => c.argv[0] === "feedback"), [], `${prompt} aborted=${aborted}`);
  }
});

test("dismissing the question (no answer) sends nothing", async () => {
  const h = offerHarness({ answer: undefined });
  await h.run("change a", [["edit", "a.go"]]);
  assert.equal(h.asked.length, 1);
  assert.deepEqual(h.sent, []);
});

test("pytest and Go test files never count as source to offer TSP for", () => {
  for (const p of ["lib/x_test.py", "lib/test_x.py", "pkg/x_test.go", "src/a.test.ts"]) assert.equal(isSourcePath(p), false, p);
  for (const p of ["lib/attest.py", "cmd/latest.go"]) assert.equal(isSourcePath(p), true, p);
});

test("files under the test directories the Stop gate names never trigger the TSP offer", () => {
  for (const p of ["e2e/login.ts", "spec/user_spec.rb", "app/spec/models/user.rb"]) assert.equal(isSourcePath(p), false, p);
});

test("/tsp check keeps a quoted argument as one word", () => {
  assert.deepEqual(parseTppCommand(`check --suite "npm test" --path 'docs/my plan.md'`), { kind: "check", rest: ["--suite", "npm test", "--path", "docs/my plan.md"] });
  assert.equal(parseTppCommand(`check --suite "npm test`).kind, "help");
});

test("a run that only wrote a report never runs the check: no production source, no debt", async () => {
  // The report case: the work produced documents, so nothing this run touched owes a test and no question appears.
  const h = offerHarness();
  await h.run("write the review report", [["write", "reports/2026-10-review.md"], ["edit", "notes/plan.md"]]);
  assert.deepEqual(h.calls, [], "a report is not an implementation");
  assert.deepEqual(h.asked, []);
  assert.deepEqual(h.sent, []);
});
