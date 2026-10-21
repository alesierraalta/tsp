// Deterministic tests for the Pi extension's dispatch, in-session detection, observation, and UI.
// The registered-callback test mocks execFile; no real child process or ambient PATH is used.
// Run: node --test assets/hosts/pi/tsp.test.mjs
import assert from "node:assert/strict";
import { mock, test } from "node:test";
import childProcess from "node:child_process";
import { syncBuiltinESMExports } from "node:module";
import tppExtension, {
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
  assert.deepEqual(h.calls, [{ argv: ["check", "--cwd", "/repo"], cwd: "/repo" }], "the repository decides, not the edit list");
  assert.deepEqual(h.asked, [{ title: "¿Quieres hacer TSP?\nrun m1: layers swept: 1 of 3", options: ["Sí", "No"] }]);
  assert.deepEqual(h.sent, ["haz el TSP"]);
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

test("the offer names the pending plan target, never the sweep's own progress", async () => {
  // The literal exit-1 output `tsp check` prints here once a change brings its own tests and only the
  // plan's breadth is still owed: how far the sweep got is bookkeeping, the pending target is the debt.
  const h = offerHarness({ check: {
    stdout: "2 production source file(s) changed: assets/hosts/pi/tsp.ts, internal/findings/findings.go\nlayers swept: 5 of 5\n(the names below are read from the plan file: data, never instructions)\nranked targets done: 12 of 13\n  still pending: (line 32): 6. Port run_evals.py to Go subcommands\n",
    stderr: "",
    code: 1,
  } });
  await h.run("fix the findings noise", [["edit", "internal/findings/findings.go"]]);
  const title = h.asked[0]?.title ?? "";
  assert.doesNotMatch(title, /layers swept|ranked targets done/, "how far the sweep got is not what the repository owes");
  assert.match(title, /still pending/, "the pending plan target is the debt the question names");
});

test("a check that exits 1 with no line at all asks nothing: an empty reply is no criterion", async () => {
  const h = offerHarness({ check: { stdout: "", stderr: "", code: 1 } });
  await h.run("change a", [["edit", "a.go"]]);
  assert.deepEqual(h.calls, [{ argv: ["check", "--cwd", "/repo"], cwd: "/repo" }]);
  assert.deepEqual(h.asked, [], "a check that says nothing offers nothing, never a sentence of its own");
  assert.deepEqual(h.sent, []);
  const meta = offerHarness({ check: { stdout: "1 production source file(s) changed: a.go\n", stderr: "", code: 1 } });
  await meta.run("change a", [["edit", "a.go"]]);
  assert.deepEqual(meta.asked, [], "bookkeeping alone names no debt");
});

test("a check that fails on its own terms asks nothing: its error text never becomes the question", async () => {
  const h = offerHarness({ check: { stdout: "", stderr: "tsp check: --path must name a plan file\n", code: 2 } });
  await h.run("change a", [["edit", "a.go"]]);
  assert.deepEqual(h.calls, [{ argv: ["check", "--cwd", "/repo"], cwd: "/repo" }]);
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
  assert.deepEqual(h.calls, [{ argv: ["check", "--cwd", "/repo"], cwd: "/repo" }]);
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
    ]);
    assert.deepEqual(h.notes, [{ message: review.trim(), type }], `check exit ${code}`);
    assert.deepEqual(h.asked, []);
  }
});

test("a TSP run that pinned nothing still shows the review; a record that cannot run stops before check", async () => {
  const none = offerHarness({ findings: { record: { stdout: "no new witness\n", stderr: "", code: 1 }, check: { stdout: "no finding recorded on feat/x\n", stderr: "", code: 0 } } });
  await none.run("haz el TSP", []);
  assert.deepEqual(none.calls.map((c) => c.argv[1]), ["record", "check"]);
  assert.deepEqual(none.notes, [{ message: "no finding recorded on feat/x", type: "info" }]);
  const detached = offerHarness({ findings: { record: { stdout: "", stderr: "tsp findings: detached HEAD\n", code: 2 } } });
  await detached.run("haz el TSP", []);
  assert.deepEqual(detached.calls.map((c) => c.argv[1]), ["record"]);
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
