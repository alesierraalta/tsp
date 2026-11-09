// tsp for Pi: the shared tsp CLI as a native /tsp command, loadable without an installer:
//   pi --extension /path/to/assets/hosts/pi/tsp.ts
//
// Detection reads this session's registered tools (pi.getAllTools()), never PATH: exact
// Gentle-owned tool names or whole gentle-pi/gentle-ai source path segments. When Gentle is
// observed, the spawned `tsp doctor` child gets the process-scoped observation through
// child_process.execFile's env — no process.env mutation, no file. That observation is session
// UX evidence for mode selection, not authentication or security authority. Gentle review
// authority is never called; UI writes are guarded by ctx.hasUI.

import { execFile, execFileSync } from "node:child_process";
import { createHash } from "node:crypto";
import { existsSync, readFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import type { ExtensionAPI, ExtensionCommandContext, ExtensionContext } from "@earendil-works/pi-coding-agent";

/** Protocol with internal/mode (Go); mode.VerifiedGentleSignal accepts only this exact pair. */
export const GENTLE_OBSERVATION_ENV = "TPP_GENTLE_OBSERVATION";
export const GENTLE_OBSERVATION_VALUE = "pi-session-gentle-active";

export interface RegisteredTool {
  name: string;
  sourceInfo?: { path?: string };
}

export type TppDispatch =
  | { kind: "check"; rest: string[] } | { kind: "feedback-summary" }
  | { kind: "doctor"; mode: "auto" | "gentle" } | { kind: "run" } | { kind: "help"; reason: string };

// The Gentle-owned tools registered by gentle-pi, by exact name — not any gentle_* prefix.
const GENTLE_TOOLS = new Set(["gentle_odd_phase", "gentle_review", "gentle_review_capture", "gentle_review_capture_group", "gentle_review_scope"]);
// Gentle-owned directories, matched as whole path segments only — never as substrings.
const GENTLE_DIRS = new Set(["gentle-pi", "gentle-ai"]);

/** True when Gentle is actually registered in this session — no PATH lookup, no version probe. */
export function observesGentleSession(tools: readonly RegisteredTool[]): boolean {
  return tools.some((t) => GENTLE_TOOLS.has(t.name) || (t.sourceInfo?.path ?? "").split(/[\\/]+/).some((s) => GENTLE_DIRS.has(s)));
}

const HELP = "Usage: /tsp run · /tsp check [--path <plan.md>] [--suite \"<test command>\"] [--no-test-needed \"<reason>\"] · /tsp feedback --summary · /tsp doctor [--mode auto|gentle]";

/** Splits arguments on whitespace, keeping a single- or double-quoted span as one word; null when a quote is left open. */
function splitArgs(raw: string): string[] | null {
  const words: string[] = [];
  let word = "", quote = "", inWord = false;
  for (const ch of raw) {
    if (quote) {
      if (ch === quote) quote = "";
      else word += ch;
    } else if (ch === '"' || ch === "'") {
      quote = ch;
      inWord = true;
    } else if (/\s/.test(ch)) {
      if (inWord) words.push(word);
      word = "";
      inWord = false;
    } else {
      word += ch;
      inWord = true;
    }
  }
  if (quote) return null;
  if (inWord) words.push(word);
  return words;
}

/** Maps the raw arguments after /tsp or its /tpp compatibility alias. */
export function parseTppCommand(rawArgs: string): TppDispatch {
  const words = splitArgs(rawArgs);
  if (words === null) return { kind: "help", reason: "unclosed quote" };
  const [head, ...rest] = words;
  switch (head) {
    case undefined: return { kind: "help", reason: "no subcommand" };
    case "check": return { kind: "check", rest };
    case "run": return rest.length === 0 ? { kind: "run" } : { kind: "help", reason: "run takes no arguments" };
    case "feedback":
      return rest.length === 1 && rest[0] === "--summary" ? { kind: "feedback-summary" } : { kind: "help", reason: "feedback supports only --summary" };
    case "doctor": {
      if (rest.length === 0) return { kind: "doctor", mode: "auto" };
      const mode = rest[1];
      return rest.length === 2 && rest[0] === "--mode" && (mode === "auto" || mode === "gentle")
        ? { kind: "doctor", mode } : { kind: "help", reason: "doctor supports only [--mode auto|gentle]" };
    }
    default: return { kind: "help", reason: `unknown subcommand "${head}"` };
  }
}

/** The shared CLI's argv for one dispatch. */
export function buildArgv(d: TppDispatch, cwd: string): string[] {
  switch (d.kind) {
    case "check": return ["check", "--cwd", cwd, ...d.rest];
    case "feedback-summary": return ["feedback", "--summary"];
    case "doctor": return ["doctor", "--mode", d.mode];
    case "run": return [];
    case "help": return [];
  }
}

/**
 * Child env for one tpp process: the observation rides an observed doctor or check child, the two that decide from it
 * (check holds a Gentle session to the required gate, #418); base is copied, never mutated.
 */
export function childEnv(base: Readonly<Record<string, string | undefined>>, d: TppDispatch, gentleObserved: boolean): Record<string, string | undefined> {
  const env: Record<string, string | undefined> = { ...base };
  if (gentleObserved && (d.kind === "doctor" || d.kind === "check")) env[GENTLE_OBSERVATION_ENV] = GENTLE_OBSERVATION_VALUE;
  return env;
}

type ExecResult = { stdout: string; stderr: string; code: number };
type ExecOpts = { cwd: string; env: Record<string, string | undefined> };
export type ExecTpp = (argv: readonly string[], opts: ExecOpts) => Promise<ExecResult>;

// Node's default 1 MiB would kill a long answer with "stdout maxBuffer length exceeded"; notices truncate it anyway.
const MAX_CHILD_OUTPUT = 64 * 1024 * 1024;

/** One tpp child without a shell; a non-zero exit is data — only a binary that never started rejects. */
export function execTpp(argv: readonly string[], opts: ExecOpts): Promise<ExecResult> {
  return new Promise((resolve, reject) => {
    execFile("tsp", [...argv], { cwd: opts.cwd, env: opts.env as NodeJS.ProcessEnv, encoding: "utf8", maxBuffer: MAX_CHILD_OUTPUT }, (error, stdout, stderr) => {
      if (error && typeof (error as { code?: unknown }).code !== "number") return reject(error);
      resolve({ stdout: String(stdout), stderr: String(stderr), code: error ? Number((error as { code?: unknown }).code) : 0 });
    });
  });
}

export interface TppDeps {
  tools: readonly RegisteredTool[];
  cwd: string;
  env: Readonly<Record<string, string | undefined>>;
  hasUI: boolean;
  exec: ExecTpp;
  notify: (message: string, type?: "info" | "warning" | "error") => void;
  setStatus?: (text: string | undefined) => void; // status-bar writer; called only when hasUI
  startRun?: () => void; // `/tsp run`: the same request the offer sends when it is accepted
  show?: (text: string | undefined) => void; // panel writer for a report a toast cannot hold
}

const MAX_NOTICE = 1200; // a notification is a toast, not a terminal
const notice = (text: string): string => (text.trim().length <= MAX_NOTICE ? text.trim() : text.trim().slice(0, MAX_NOTICE) + "…");

/** Runs one invocation; every render is guarded by hasUI so headless sessions still get the child process. */
export async function runTppCommand(rawArgs: string, deps: TppDeps): Promise<void> {
  const d = parseTppCommand(rawArgs);
  if (d.kind === "help") {
    if (deps.hasUI) deps.notify(`${d.reason}. ${HELP}`, "warning");
    return;
  }
  if (d.kind === "run") {
    // A delegated child has nobody to run for, and the user still deserves the reason.
    if (deps.env.GENTLE_PI_AGENTS_CHILD === "1") { deps.notify(notice("tsp run: this is a delegated subagent session, so it starts no TSP run"), "warning"); return; }
    deps.startRun?.();
    return;
  }
  const observed = observesGentleSession(deps.tools);
  const label = d.kind === "feedback-summary" ? "feedback" : d.kind;
  let outcome: ExecResult;
  try {
    outcome = await deps.exec(buildArgv(d, deps.cwd), { cwd: deps.cwd, env: childEnv(deps.env, d, observed) });
  } catch (error) {
    if (deps.hasUI) {
      deps.notify(notice(`tsp: ${error instanceof Error ? error.message : String(error)}`), "error");
      deps.setStatus?.("tsp: failed");
    }
    return;
  }
  if (deps.hasUI) {
    const output = outcome.stdout.trim() || outcome.stderr.trim();
    const ok = outcome.code === 0;
    const full = output || `tsp ${label}: ${ok ? "ok" : `exited ${outcome.code}`}`;
    // A toast is a summary: a report too long for it is shown whole in the panel, and the toast says where.
    if (deps.show !== undefined && full.length > MAX_NOTICE) {
      deps.show(full);
      deps.notify(`tsp ${label}: ${ok ? "ok" : `exit ${outcome.code}`} — the full output is in the panel`, ok ? "info" : "error");
    } else {
      deps.notify(notice(full), ok ? "info" : "error");
    }
    const suffix = observed && d.kind === "doctor" ? " · gentle session observed" : "";
    deps.setStatus?.(`tsp ${label}: ${ok ? "ok" : `exit ${outcome.code}`}${suffix}`);
  }
}

// The TSP offer. When a run ends after the agent edited source code, and the repository still owes that change
// a test, Pi runs TSP on its own when the repository says so and asks in the chat when the repository left the
// decision open; "Sí" sends "haz el TSP" as if the user typed it, which invokes the test-strategy skill. Only edit
// and write results are seen, so a change made through a shell command is not noticed. `tsp check --json` is the
// criterion and the policy: it answers for the paths this run touched AND for the commit the turn started from,
// because a turn that commits its work unit leaves a clean tree the check alone would read as nothing changed.
// Its verdict names the decision — required means the repository declared the run, ask means it left that to the
// human, none or an error means the offer stays silent and Pi never asks a question whose answer is already known.
// When a TSP run ends, the tests it left failing are recorded as findings and `tsp findings next` decides the loop:
// fix sends the agent what to settle, tsp sends the next round, done and stop end it; the user sees the review and
// the answer every time. After a fix run only `next` is asked. The agent is also asked to record its feedback on the
// template `tsp feedback --template` prints, so each run leaves a report later runs can be tuned on, unless the run
// already recorded one (a `tsp feedback --file` whose output says it was recorded), which would count it twice; the loop's
// message waits until that feedback run ends, and an aborted turn or the user's own message drops it. The feedback
// request is a run of its own: it reviews no findings, asks no TSP question and requests no more feedback.
// A delegated child session (GENTLE_PI_AGENTS_CHILD=1) has nobody to answer, so it gets none of them.
export const OFFER_QUESTION = "¿Quieres hacer TSP?";
export const OFFER_YES = "Sí";
export const OFFER_NO = "No";
export const TSP_PROMPT = "haz el TSP";
export const FEEDBACK_LEAD = "The TSP run has ended. Record honest feedback on it.";
export const FIX_LEAD = "The TSP loop found defects to settle.";
export const ROUND_LEAD = `${TSP_PROMPT} — `;

const SOURCE_EXT = /\.(go|ts|tsx|js|jsx|mjs|cjs|py|rs|java|kt|rb|php|cs|c|cc|cpp|h|hpp|swift|scala|sh)$/;
const TEST_PATH = /(^|\/)(tests?|__tests__|testdata|e2e|spec)\/|_test\.(go|py)$|\.(test|spec)\.[^/]+$|(^|\/)test_[^/]+\.py$/;

/**
 * What a TSP run owes the plan when it wrote tests and recorded no finding (#398). The plan is the only thing a
 * later reader sees, so a run that pinned a defect and left no Findings row loses it; the notice asks for the row
 * once, and names the check that proves the witness fails on this code.
 */
export function unrecordedRunNotice(findings: number, changed: readonly string[]): string {
  if (findings > 0) return "";
  if (!changed.some((path) => TEST_PATH.test(path))) return "";
  return "This TSP run added or changed a test file and the plan has no Findings row for what it found, so the finding is lost to every reader of the plan: record each defect with `tsp plan add-finding` (its `path:line`, severity, evidence and the test that fails on this code) and confirm the witness with `tsp plan check --pins`.";
}

/** True for a source file the agent may have changed behaviour in; tests, docs and config are not. */
export function isSourcePath(path: string): boolean {
  const p = path.replace(/\\/g, "/");
  return SOURCE_EXT.test(p) && !TEST_PATH.test(p);
}

/**
 * A TSP run is a run whose agent loads the test-strategy skill, whatever words started it: the skill's trigger list is
 * long and a message that only mentions tsp starts none. Pi loads a skill three ways: the read tool on its SKILL.md,
 * bash when that is the configured file reader, or a /skill:test-strategy command, which arrives as a <skill> block
 * leading the prompt. The exact request the offer sends is a TSP run too, because a second TSP in the same session may
 * run on the skill the agent already holds without reading it again. Its own edits never trigger another offer.
 *
 * Only loading counts. The file must be SKILL.md itself, not a sibling that starts with its name; a copy inside the
 * working repository (a relative path, or one under the working directory outside its .pi/ folder) is source being
 * worked on, not a skill being loaded; and a bash command counts only when it reads the file, not when it names it.
 */
const SKILL_FILE = /(^|[\/\\])test-strategy[\/\\]SKILL\.md$/;
const SKILL_BLOCK = /^<skill name="test-strategy" /;
const FILE_READER = /^\s*(cat|head|tail|less|more|bat|sed)\s/;
const isTspRequest = (prompt: string | undefined): boolean =>
  SKILL_BLOCK.test(prompt ?? "") || (prompt ?? "").trim().toLowerCase() === TSP_PROMPT.toLowerCase();
/** Which of its own messages the extension last sent: only that exact text is a feedback run, a fix run or a round. */
type OwnMessage = { prompt: string; kind: "feedback" | "fix" | "round" | "record" };
const loopKind = (message: string): OwnMessage["kind"] => (message.startsWith(FIX_LEAD) ? "fix" : "round");
function isLoadedSkill(path: string, cwd: string): boolean {
  if (!SKILL_FILE.test(path)) return false;
  const p = path.replace(/\\/g, "/");
  if (!(p.startsWith("/") || p.startsWith("~") || /^[A-Za-z]:\//.test(p))) return false;
  const root = cwd.replace(/\\/g, "/").replace(/\/+$/, "");
  return !p.startsWith(`${root}/`) || p.startsWith(`${root}/.pi/`);
}
function loadsTspSkill(toolName: string, input: unknown, cwd: string): boolean {
  const { path, command } = (input ?? {}) as { path?: unknown; command?: unknown };
  if (toolName === "read") return typeof path === "string" && isLoadedSkill(path, cwd);
  if (toolName !== "bash" || typeof command !== "string" || !FILE_READER.test(command)) return false;
  return command.split(/\s+/).map((t) => t.replace(/^['"]|['"]$/g, "")).some((t) => isLoadedSkill(t, cwd));
}

// A shell command that submits a feedback report: `tsp feedback --file <path>`, through any path to the binary.
const FEEDBACK_SUBMIT = /(?:^|[\s/;&|(])tsp\s+feedback\b[^;&|\n]*\s--file(?:[\s=]|$)/;
// What `tsp feedback --file` prints once the report is stored. A pipe can hide tsp's exit status, so the command
// counts only when its output carries this line.
const FEEDBACK_RECORDED = /^recorded (?:paid|partly|ceremony|blocked) feedback for /m;

function submitsFeedback(toolName: string, input: unknown, content: unknown): boolean {
  const { command } = (input ?? {}) as { command?: unknown };
  if (toolName !== "bash" || typeof command !== "string" || !FEEDBACK_SUBMIT.test(command)) return false;
  const text = Array.isArray(content) ? content.map((c) => (c && typeof c.text === "string" ? c.text : "")).join("\n") : "";
  return FEEDBACK_RECORDED.test(text);
}

/**
 * What the check decided about the source this run touched, or undefined when it decided nothing: the offer's whole
 * criterion and policy in one object. The edit list only says the agent touched source; the check answers for those
 * paths alone, so a plan target still open from earlier work is never mistaken for a debt this run created, and it
 * reads the commit this turn started from as well, so a turn that committed its work unit is still answered for.
 * `decision` is required, ask or none; anything else — an error verdict, a reply that is not one object, a child that
 * cannot run — yields no criterion and the offer stays silent.
 * A tsp older than this extension refuses the flag itself; that is not a verdict and not silence either, so it is
 * reported as `stale` and the caller says so once instead of going quiet about an install that cannot answer.
 */
async function checkVerdict(cwd: string, env: Readonly<Record<string, string | undefined>>, exec: ExecTpp, paths: readonly string[], since: string): Promise<Verdict | "stale" | undefined> {
  let result: ExecResult;
  try {
    // One --paths per path: a file name may itself carry a comma, and the check takes the flag whole
    // rather than splitting it, so folding the list into one value would hide exactly that file.
    const argv = ["check", "--cwd", cwd, "--json", ...(since === "" ? [] : ["--since", since]), ...paths.flatMap((p) => ["--paths", p])];
    result = await exec(argv, { cwd, env: { ...env } });
  } catch {
    return undefined;
  }
  let reply: unknown;
  try {
    reply = JSON.parse(result.stdout.trim());
  } catch {
    return STALE_CHECK.test(`${result.stdout}\n${result.stderr}`) ? "stale" : undefined;
  }
  const { decision, debt, reason } = (reply ?? {}) as { decision?: unknown; debt?: unknown; reason?: unknown };
  if (decision !== DECISION_NONE && decision !== DECISION_ASK && decision !== DECISION_REQUIRED && decision !== DECISION_ERROR) return undefined;
  // A debt that was named but does not survive reading is not the same as one never named: the first asks
  // nothing, because the check did find something a human cannot read; the second falls back to the level's reason.
  const named = typeof debt === "string" && debt.trim() !== "";
  return {
    decision,
    named,
    debt: named ? reasonOf(debt) : "",
    reason: typeof reason === "string" ? reasonOf(reason) : "",
  };
}

// What a tsp that predates `check --json` prints instead of a verdict: the flag parser's own words, never a check's.
const STALE_CHECK = /flag provided but not defined|unknown flag|invalid option/i;

// The automatic run is a toggle, not a verdict: `required` names the debt, and this says whether the
// extension may act on it without asking. An unreadable answer keeps the shipped behaviour (act).
const AUTO_RUN_FEATURE = "auto-run";
async function autoRunEnabled(cwd: string, env: Readonly<Record<string, string | undefined>>, exec: ExecTpp): Promise<boolean> {
  let result: ExecResult;
  try {
    result = await exec(["status", "--json"], { cwd, env: { ...env } });
  } catch {
    return true;
  }
  let parsed: unknown;
  try {
    parsed = JSON.parse(result.stdout.trim());
  } catch {
    return true;
  }
  const report = (parsed ?? {}) as { features?: unknown };
  if (!Array.isArray(report.features)) return true;
  const entry = report.features.find((item) => (item as { id?: unknown }).id === AUTO_RUN_FEATURE) as { enabled?: unknown } | undefined;
  return entry === undefined ? true : entry.enabled === true;
}

export interface Verdict {
  decision: typeof DECISION_NONE | typeof DECISION_ASK | typeof DECISION_REQUIRED | typeof DECISION_ERROR;
  named: boolean; // the verdict named a debt at all
  debt: string; // what this change owes, ready to show
  reason: string; // what decided the level
}
const DECISION_NONE = "none";
const DECISION_ASK = "ask";
const DECISION_REQUIRED = "required";
const DECISION_ERROR = "error";

/**
 * The reason the question shows, or an empty string when it would show nothing. The line can quote plan-file text
 * verbatim, so it is stripped of control characters and cut at MAX_REASON characters, an ellipsis marking the cut; the
 * cap sits above the longest line the check prints, so its own wording reaches the human whole and only plan text runs
 * into it. The cut counts characters, not their UTF-16 halves, so it never leaves a lone surrogate to render.
 *
 * The visibility question is asked last, on the very text that will be shown. Asking it of the line as printed lets
 * the cut delete the only character that painted, or add an ellipsis that fakes paint, and either way the question
 * would name a line nobody can read while hiding the debt printed after it.
 */
const MAX_REASON = 300;
function reasonOf(line: string): string {
  const chars = [...line.replace(/[\u0000-\u001f\u007f-\u009f]/g, "")];
  const cut = chars.length > MAX_REASON;
  const kept = (cut ? chars.slice(0, MAX_REASON - 1) : chars).join("");
  if (!paints(kept)) return "";
  return cut ? `${kept}…` : kept;
}

/**
 * Text paints when it carries a letter, a digit or punctuation. A line of controls, formatting characters or bare
 * glyph shapes renders as nothing, and listing the invisible categories to exclude always leaves one out, so the
 * question is asked the positive way. The Hangul fillers are letters that paint nothing, so they are not paint.
 */
function paints(line: string): boolean {
  return /[\p{L}\p{N}\p{P}]/u.test(line.replace(/[\u115F\u1160\u3164\uFFA0]/g, ""));
}

export interface OfferOptions {
  optedOut?: (cwd: string) => boolean; // the repo's .no-testing-gate opt-out, shared with the Stop gate
  env?: Readonly<Record<string, string | undefined>>; // defaults to process.env
  exec?: ExecTpp; // defaults to the tsp binary
  head?: (cwd: string) => string; // the commit a turn starts from; defaults to asking git
  probe?: RepoProbe; // the working tree outside the edit tools; defaults to asking git
}

/** One reading of the working tree: the commit at its head and a digest per source file it has changed. */
export interface RepoSnapshot { head: string; files: ReadonlyMap<string, string> }

/**
 * What the offer reads besides the edit tools: the source a shell command changed, and a work unit this turn
 * committed (which leaves no worktree change to see). Undefined means the repository could not be read.
 */
export interface RepoProbe {
  state(cwd: string): RepoSnapshot | undefined;
  committed(cwd: string, from: string, to: string): readonly string[];
}

/** Runs git outside the checkout, for the same reason headAt does. */
function gitOut(cwd: string, args: readonly string[]): string | undefined {
  try {
    return execFileSync("git", ["-C", cwd, ...args], {
      cwd: tmpdir(), encoding: "utf8", stdio: ["ignore", "pipe", "ignore"], windowsHide: true,
    });
  } catch {
    return undefined;
  }
}

/** The file's content, or "" when it is gone: a digest tells a turn's change from a file that was already dirty. */
function digest(path: string): string {
  try {
    return createHash("sha1").update(readFileSync(path)).digest("hex");
  } catch {
    return "";
  }
}

/**
 * A porcelain status path, read from the turn's own directory. Porcelain paths are relative to the repository
 * root, while --paths reads from the directory the turn runs in; a change outside that directory is not the
 * turn's. The empty prefix means the turn runs at the root, where both are the same.
 */
export function cwdPath(prefix: string, path: string): string | undefined {
  if (prefix === "") return path;
  return path.startsWith(prefix) ? path.slice(prefix.length) : undefined;
}

export const repoProbe: RepoProbe = {
  state(cwd) {
    // Porcelain status is always root-relative, whatever a user config says: read the turn's own prefix first.
    const prefix = gitOut(cwd, ["rev-parse", "--show-prefix"]);
    if (prefix === undefined) return undefined;
    const out = gitOut(cwd, ["status", "--porcelain=v1", "-z", "--untracked-files=all"]);
    if (out === undefined) return undefined;
    const files = new Map<string, string>();
    const parts = out.split("\0");
    for (let i = 0; i < parts.length; i++) {
      const entry = parts[i];
      if (entry.length < 4) continue;
      const status = entry.slice(0, 2);
      // A rename or copy carries the source path in the next field, which is not a change of its own.
      if (status[0] === "R" || status[0] === "C") i++;
      const path = cwdPath(prefix.trim(), entry.slice(3));
      // Every changed path is kept, tests included: the offer filters source paths itself, and a test the turn wrote
      // is what a finding has to be recorded for (#398).
      if (path !== undefined) files.set(path, digest(join(cwd, path)));
    }
    return { head: headAt(cwd), files };
  },
  committed(cwd, from, to) {
    const out = gitOut(cwd, ["diff", "--name-only", "--relative", `${from}..${to}`]);
    if (out === undefined) return [];
    return out.split("\n").map((line) => line.trim()).filter((path) => path !== "");
  },
};

/** The commit this turn starts from, or "" when it cannot be read: a repository with no commit yet owes nothing. */
export function headAt(cwd: string): string {
  try {
    // Run outside the checkout and name the repository with -C: a child process is located through its
    // own working directory on Windows, so resolving git from inside a checkout would let the checkout
    // decide which git answers merely because a turn started in it.
    const out = execFileSync("git", ["-C", cwd, "rev-parse", "HEAD"], {
      cwd: tmpdir(), encoding: "utf8", stdio: ["ignore", "pipe", "ignore"], windowsHide: true,
    }).trim();
    return /^[0-9a-f]{40}$|^[0-9a-f]{64}$/.test(out) ? out : "";
  } catch {
    return "";
  }
}

type Notify = (message: string, type?: "info" | "warning" | "error") => void;
const REVIEW_TYPE = { 0: "info", 1: "warning" } as const;

/** The loop the branch is in, as the status line shows it: what it owes next, how far it got, what is open. */
export interface LoopStatus { next: string; round: number; maxRounds: number; open: number }

/** What the status line shows: the loop the branch is in, else the debt the last check named; undefined clears it. */
export function statusFor(decision: string | undefined, loop: LoopStatus | undefined): string | undefined {
  if (loop !== undefined && (loop.next === "fix" || loop.next === "tsp")) {
    const round = loop.round > 0 && loop.maxRounds > 0 ? ` · round ${loop.round}/${loop.maxRounds}` : "";
    const open = loop.open > 0 ? ` · ${loop.open} open` : "";
    return `tsp: ${loop.next}${round}${open}`;
  }
  if (decision === "ask" || decision === "required") return `tsp: ${decision}`;
  return undefined;
}

/** What the offer remembers on the session branch: a reload, a resume or a fork rebuilds exactly this. */
interface OfferState { declined: string[]; lastFix: string; pending: string; ran: boolean; own?: OwnMessage; loop?: LoopStatus }
const STATE_TYPE = "tsp-state";

/** What the loop sends the agent next ("" for nothing) and, for a fix answer, the tree and message that identify it. */
export interface LoopStep { message: string; fix: string; loop?: LoopStatus }
const NO_STEP: LoopStep = { message: "", fix: "" };

/**
 * Records the TSP run's witnesses (a record that cannot run stops there), asks `tsp findings next` what the loop owes,
 * shows the user the review, the answer and its notes, and returns the step the agent is sent next. A tsp without
 * `findings next` gets today's review instead and the loop stays manual.
 */
/** How many Findings rows the repository's plan holds, or undefined when it cannot say. */
export async function planFindingCount(cwd: string, env: Readonly<Record<string, string | undefined>>, exec: ExecTpp): Promise<number | undefined> {
  try {
    const r = await exec(["plan", "count", "--json"], { cwd, env: { ...env } });
    if (r.code !== 0) return undefined;
    const parsed = JSON.parse(r.stdout.trim()) as { findings?: unknown };
    return typeof parsed.findings === "number" ? parsed.findings : undefined;
  } catch { return undefined; }
}

export async function reviewFindings(cwd: string, env: Readonly<Record<string, string | undefined>>, exec: ExecTpp, notify: Notify, record = true, show?: (text: string | undefined) => void): Promise<LoopStep> {
  const run = (sub: string, args: string[] = []) => exec(["findings", sub, "--cwd", cwd, ...args], { cwd, env: { ...env } });
  const shown = (r: ExecResult) => notice(r.stdout.trim() || r.stderr.trim() || `tsp findings: exited ${r.code}`);
  // A review too long for a toast is shown whole in the panel; the toast keeps the verdict and says where it is.
  const showOrNotify = (text: string, type: "info" | "warning" | "error") => {
    if (show !== undefined && text.trim().length > MAX_NOTICE) { show(text); notify(`tsp findings: the whole review is in the panel`, type); return; }
    notify(notice(text), type);
  };
  try {
    if (record) {
      const recorded = await run("record");
      if (recorded.code !== 0 && recorded.code !== 1) { notify(shown(recorded), "error"); return NO_STEP; }
    }
    const result = await run("next", ["--json"]);
    let parsed: unknown;
    try { parsed = JSON.parse(result.stdout.trim()); } catch { parsed = null; }
    const reply = parsed && !Array.isArray(parsed) && typeof parsed === "object" ? parsed as { next?: unknown; review?: unknown; reason?: unknown; message?: unknown; notes?: unknown; tree?: unknown; lines?: unknown; round?: unknown; max_rounds?: unknown; items?: unknown } : null;
    if (!reply || typeof reply.next !== "string" || !["fix", "tsp", "done", "stop", "error"].includes(reply.next)) {
      // `next` documents exit 2 as "cannot judge", and a binary that predates the subcommand answers with its
      // usage text instead. Both exit 2, so the answer decides: a usage reply is compatibility and keeps the
      // plain review, while anything else is a judgment that failed, and a failed judgment must never read as
      // "the loop found nothing to do" and end the loop without a word.
      const review = await run("check");
      const shownReview = shown(review);
      if (/^usage (of findings|: tsp)/i.test(result.stderr.trim())) {
        showOrNotify(shownReview, REVIEW_TYPE[review.code as 0 | 1] ?? "error");
        return NO_STEP;
      }
      const diagnostic = [result.stderr.trim(), result.stdout.trim()].filter(Boolean).join("\n");
      const why = diagnostic === "" ? `tsp findings next: exited ${result.code} with no answer` : `tsp findings next: ${diagnostic}`;
      showOrNotify(`${why}\n${shownReview}`, "error");
      return NO_STEP;
    }
    const list = (value: unknown): unknown[] => (Array.isArray(value) ? value : []);
    const message = typeof reply.message === "string" ? reply.message : "";
    // An end sends the agent nothing more, so its whole message (the rejected findings, the items the user has to
    // decide on) is shown; a fix or a round shows its reason and notes, and its message goes to the agent.
    const terminal = reply.next === "done" || reply.next === "stop" || reply.next === "error";
    const shownLines = [reply.review, ...list(reply.lines), ...(terminal ? [message] : [reply.reason, ...list(reply.notes)])];
    const count = (value: unknown): number => (typeof value === "number" && Number.isFinite(value) && value > 0 ? value : 0);
    const loop: LoopStatus = { next: reply.next, round: count(reply.round), maxRounds: count(reply.max_rounds), open: list(reply.items).length };
    showOrNotify(shownLines.filter((s): s is string => typeof s === "string" && s !== "").join("\n"), reply.next === "stop" || reply.next === "error" ? "warning" : "info");
    if (reply.next === "fix") {
      const tree = typeof reply.tree === "string" ? reply.tree : "";
      return { message: `${FIX_LEAD}\n\n${message}`, fix: tree === "" ? "" : `${tree}\n${message}`, loop };
    }
    // The loop ended: nothing is running any more, so the panel that held its review is cleared.
    if (terminal) show?.(undefined);
    return { message: reply.next === "tsp" ? `${ROUND_LEAD}${message}` : "", fix: "", loop: terminal ? undefined : loop };
  } catch (error) {
    notify(notice(`tsp: ${error instanceof Error ? error.message : String(error)}`), "error");
    return NO_STEP;
  }
}

/**
 * The feedback request for the TSP run that just ended, or "" when there is nothing to ask. The template is asked of
 * the positive way: a reply that carries the `verdict:` field is a template, so the line tsp prints while feedback is
 * disabled, an empty reply or a failing tsp never costs the agent a run. Feedback switched off is the user's choice
 * and stays quiet; any other reply that is not a template is a request that never went out, so the user is told why.
 */
export async function feedbackRequest(cwd: string, env: Readonly<Record<string, string | undefined>>, exec: ExecTpp, notify: Notify): Promise<string> {
  try {
    const result = await exec(["feedback", "--template"], { cwd, env: { ...env } });
    const template = result.stdout.trim();
    if (result.code === 0 && template.startsWith("feedback is disabled")) return "";
    if (result.code !== 0 || !/^verdict:/m.test(template)) {
      const why = result.stderr.trim() || (result.code !== 0 ? `tsp feedback: exited ${result.code}` : "tsp feedback: no template");
      notify(notice(`${why} — no feedback was requested for this TSP run`), "warning");
      return "";
    }
    return `${FEEDBACK_LEAD} Fill skill (what actually ran), paid, cost, reason and verdict below from what that run actually found and what it cost ` +
      "(paid is \"nothing\" when it found nothing), save the report to a temporary file outside the repository and submit " +
      "it with `tsp feedback --file <that file>`. The skill field names the skill and version you actually read, not the " +
      "one this binary embeds. Do not change code or tests.\n\n" + template;
  } catch (error) {
    notify(notice(`tsp: ${error instanceof Error ? error.message : String(error)} — no feedback was requested for this TSP run`), "warning");
    return "";
  }
}

function registerOffer(pi: ExtensionAPI, opts: OfferOptions): () => void {
  const optedOut = opts.optedOut ?? ((cwd: string) => existsSync(join(cwd, ".no-testing-gate")));
  const env = opts.env ?? process.env;
  const exec = opts.exec ?? execTpp;
  const head = opts.head ?? headAt;
  const probe = opts.probe ?? repoProbe;
  const child = env.GENTLE_PI_AGENTS_CHILD === "1";
  // Read at each check, not at load: gentle-pi may register its tools after this extension.
  const checkEnv = () => childEnv(env, { kind: "check", rest: [] }, observesGentleSession(pi.getAllTools()));
  const edited = new Set<string>(); // source paths changed in the current run through the edit tools
  let opened: RepoSnapshot | undefined; // the working tree as this run found it, read only when the offer can fire
  const declined = new Set<string>(); // paths the user already said No to
  let turnHead = ""; // the commit this turn started from: what --since reads the work unit against
  let staleNotified = false; // a tsp too old to answer is reported once, not once per turn
  let errorNotified = false; // a policy that could not be read is reported once, without becoming debt
  let tspRun = false;
  let feedbackGiven = false; // the agent already submitted this run's feedback itself, so asking again counts it twice
  let feedbackRun = false;
  let fixRun = false;
  let pending = "";
  let lastFix = ""; // the fix answer the agent was last sent: the same one again after its fix run means it changed nothing
  let ran = false; // a TSP run already happened on this branch: one run per branch, never one per turn
  let continued = false;
  let recordNudged = false; // the run that pinned a test was already asked to record it: once per session
  let pendingRecord = ""; // the recording notice waiting for a turn of its own (#398)
  let decision: string | undefined; // the debt the last check named, for the status line
  let loop: LoopStatus | undefined; // the loop the branch is in, for the status line
  // A user's message that merely starts like one of the extension's own is the user's: it never runs the loop.
  let own: OwnMessage | undefined;
  // The decisions live on the session branch, so a reload, a resume and a fork rebuild them instead of forgetting them.
  const persist = () => pi.appendEntry(STATE_TYPE, { declined: [...declined], lastFix, pending, ran, own, loop } satisfies OfferState);
  const restore = (ctx: { sessionManager?: { getBranch?: () => readonly unknown[] } }) => {
    declined.clear(); lastFix = ""; pending = ""; ran = false; own = undefined; loop = undefined; decision = undefined;
    try {
      let data: unknown;
      for (const entry of (ctx.sessionManager?.getBranch?.() ?? []) as { type?: unknown; customType?: unknown; data?: unknown }[]) {
        if (entry?.type === "custom" && entry.customType === STATE_TYPE) data = entry.data;
      }
      const s = (data !== null && typeof data === "object" ? data : {}) as Record<string, unknown>;
      if (Array.isArray(s.declined)) for (const path of s.declined) if (typeof path === "string") declined.add(path);
      if (typeof s.lastFix === "string") lastFix = s.lastFix;
      if (typeof s.pending === "string") pending = s.pending;
      if (s.ran === true) ran = true;
      const saved = s.loop as Partial<LoopStatus> | undefined;
      loop = saved !== null && typeof saved === "object" && typeof saved.next === "string" && ["fix", "tsp", "done", "stop", "error"].includes(saved.next)
        && [saved.round, saved.maxRounds, saved.open].every((n) => typeof n === "number" && Number.isFinite(n) && n >= 0)
        ? { next: saved.next, round: saved.round as number, maxRounds: saved.maxRounds as number, open: saved.open as number } : undefined;
      const sent = s.own as { prompt?: unknown; kind?: unknown } | undefined;
      if (sent !== null && typeof sent === "object" && typeof sent.prompt === "string" && (sent.kind === "feedback" || sent.kind === "fix" || sent.kind === "round")) {
        own = { prompt: sent.prompt, kind: sent.kind };
      }
    } catch {
      // State an older version wrote, or that a broken write left behind, is ignored: it never fails a session.
    }
  };
  const send = (prompt: string, kind: OwnMessage["kind"]) => {
    own = { prompt, kind };
    pi.sendUserMessage(prompt);
    persist();
  };
  pi.on("session_start", (event, ctx) => {
    restore(ctx);
    // A loop that was waiting on a run when the session was reloaded resumes here, once, instead of stalling.
    // A new session has nothing to resume, and a fork is an alternative history: it keeps the state, not the send.
    const reason = (event as { reason?: unknown }).reason;
    if (reason !== "new" && reason !== "fork" && pending !== "") { const next = pending; pending = ""; send(next, loopKind(next)); }
    if (ctx.hasUI) paint(ctx);
  });
  /** Writes the status line; a status write never fails a turn, and a host without one simply has no line. */
  const paint = (ctx: ExtensionContext) => {
    try { ctx.ui.setStatus("tsp", statusFor(decision, loop)); } catch { /* a status write never fails a turn */ }
  };
  // /tree changes the branch under the session: its entries are the state from now on.
  pi.on("session_tree", (_event, ctx) => restore(ctx));
  pi.on("before_agent_start", (event, ctx) => {
    edited.clear();
    turnHead = head(ctx.cwd);
    const user = event.messageType === "user";
    const mine = user && own !== undefined && (event.prompt ?? "").trim() === own.prompt.trim() ? own.kind : undefined;
    if (user) { if (own !== undefined) { own = undefined; persist(); } feedbackGiven = false; }
    feedbackRun = mine === "feedback";
    if (user && !feedbackRun) pending = "";
    fixRun = mine === "fix";
    tspRun = !feedbackRun && !fixRun && user && (mine === "round" || isTspRequest(event.prompt));
    // A run that can never ask, or that is the TSP itself, needs no snapshot: reading git costs a process.
    // A headless session still needs it: the repository's requirement reaches the agent through the boundary.
    opened = child || optedOut(ctx.cwd) || feedbackRun || fixRun || tspRun ? undefined : probe.state(ctx.cwd);
  });
  pi.on("tool_result", (event, ctx) => {
    if (!feedbackRun && !fixRun && !event.isError && loadsTspSkill(event.toolName, event.input, ctx.cwd)) tspRun = true;
    if (!event.isError && submitsFeedback(event.toolName, event.input, event.content)) feedbackGiven = true;
    const path = (event.input as { path?: unknown }).path;
    if ((event.toolName === "edit" || event.toolName === "write") && !event.isError && typeof path === "string") edited.add(path);
  });
  /** The source paths this run changed, from the edit tools and the working tree it opened; never filtered by a decline. */
  const collectAll = (cwd: string): string[] => {
    const fresh = new Set(edited);
    if (opened !== undefined) {
      const now = probe.state(cwd);
      if (now !== undefined) {
        for (const [path, hash] of now.files) if (opened.files.get(path) !== hash) fresh.add(path);
        // A work unit committed in this turn leaves no worktree change: the commits it made are its changes.
        if (opened.head !== "" && now.head !== "" && now.head !== opened.head) {
          for (const path of probe.committed(cwd, opened.head, now.head)) fresh.add(path);
        }
      }
    }
    return [...fresh];
  };
  /** The source paths this run changed, which is what the offer answers. */
  const collect = (cwd: string): string[] => collectAll(cwd).filter(isSourcePath);
  pi.on("agent_before_settle", async (event, ctx) => {
    // Without a UI nobody can answer the offer's question, so only the repository's own requirement runs, and it
    // reaches the agent as one continuation: an aborted run never fires this boundary, a child has nobody to run
    // for, and the branch's run or this turn's continuation already answers it.
    if (ctx.hasUI || child || continued || ran || feedbackRun || fixRun || tspRun || optedOut(ctx.cwd)) return;
    const paths = collect(ctx.cwd);
    if (paths.length === 0) return;
    const verdict = await checkVerdict(ctx.cwd, checkEnv(), exec, paths, turnHead);
    if (verdict === undefined || verdict === "stale" || verdict.decision !== DECISION_REQUIRED) return;
    continued = true;
    return {
      entries: [...((event as { entries?: readonly unknown[] }).entries ?? []), { type: "custom_message", customType: "tsp-required", content: TSP_PROMPT, display: false }],
      continue: true,
    };
  });
  const settle = async (event: { aborted: boolean }, ctx: ExtensionContext) => {
    continued = false;
    // The paths this turn changed, unfiltered: what the repository requires is read before a remembered
    // decline is applied, because a decline answers one question and cannot silence a later policy.
    const fresh = collect(ctx.cwd);
    // A test the turn wrote is what a finding has to be recorded for, and collect() keeps only source paths.
    const freshTests = collectAll(ctx.cwd).filter((p) => TEST_PATH.test(p));
    edited.clear();
    opened = undefined;
    if (event.aborted || child || optedOut(ctx.cwd)) { pending = ""; return; }
    if (pendingRecord !== "" && !tspRun && !fixRun) {
      // The notice waits for a turn of its own: sent beside the loop's message, it would read as one instruction.
      const text = pendingRecord;
      pendingRecord = "";
      if (ctx.hasUI) { ctx.ui.notify(notice("this TSP run pinned a test and recorded no finding, so the plan does not hold it"), "warning"); send(text, "record"); return; }
    }
    if (!ctx.hasUI) {
      // The boundary requested the run; a TSP run that did happen still marks the branch, so it is not requested twice.
      if (tspRun) { ran = true; persist(); }
      pending = "";
      return;
    }
    if (feedbackRun) {
      if (pending !== "") { const next = pending; pending = ""; send(next, loopKind(next)); }
      persist();
      return;
    }
    if (tspRun || fixRun) {
      const notify: Notify = (message, type) => ctx.ui.notify(message, type);
      const step = await reviewFindings(ctx.cwd, env, exec, notify, tspRun, (text) => ctx.ui.setWidget("tsp-review", text === undefined ? undefined : text.split("\n")));
      loop = step.loop;
      if (tspRun) { ran = true; decision = undefined; persist(); }
      if (fixRun && step.fix !== "" && step.fix === lastFix) {
        // The fix run left the tree and the items as they were: sending the same fix again would never end.
        lastFix = "";
        loop = undefined; // the loop stopped: nothing is running any more
        persist();
        notify(notice("the fix run changed nothing tsp findings next can see: the TSP loop stops here and needs your decision"), "warning");
        return;
      }
      lastFix = step.fix;
      persist();
      // The run that just ended may have pinned a defect and recorded nothing: the plan is what every later
      // reader sees, so the session is asked once to record it (#398).
      if (tspRun && !recordNudged) {
        const count = await planFindingCount(ctx.cwd, env, exec);
        const noticeText = count === undefined ? "" : unrecordedRunNotice(count, freshTests);
        if (noticeText !== "") { recordNudged = true; pendingRecord = noticeText; }
      }
      const request = tspRun && !feedbackGiven ? await feedbackRequest(ctx.cwd, env, exec, notify) : "";
      if (request !== "") { pending = step.message; persist(); send(request, "feedback"); return; }
      if (step.message !== "") { send(step.message, loopKind(step.message)); return; }
      // Nothing else is owed this turn, so the recording notice goes now rather than waiting for a turn that may
      // never come; when the loop did send its message, it went out on the next settle instead.
      if (pendingRecord !== "") {
        const text = pendingRecord;
        pendingRecord = "";
        notify(notice("this TSP run pinned a test and recorded no finding, so the plan does not hold it"), "warning");
        send(text, "record");
      }
      return;
    }
    const paths = fresh;
    if (paths.length === 0) return;
    // The branch's run already happened; the check still answers ask (or required) until a round is booked, and
    // repeating the run every turn is exactly the loop #342 reports. A fresh session offers again.
    if (ran) return;
    const verdict = await checkVerdict(ctx.cwd, checkEnv(), exec, paths, turnHead);
    if (verdict === "stale") {
      decision = undefined; // a check that cannot answer names no debt
      // Once per session: an install that cannot answer is worth one sentence, not one per turn.
      if (!staleNotified) {
        staleNotified = true;
        ctx.ui.notify(notice("tsp is older than this extension and cannot report a testing verdict: run `tsp update`"), "warning");
      }
      return;
    }
    if (verdict === undefined) return;
    if (verdict.decision === DECISION_ERROR) {
      decision = undefined;
      // A policy that cannot be read may mean a required run was skipped: silence would hide exactly that.
      if (!errorNotified) {
        errorNotified = true;
        const why = verdict.reason === "" ? "the repository could not say whether TSP is required" : verdict.reason;
        ctx.ui.notify(notice(`tsp check: ${why}`), "warning");
      }
      return;
    }
    if (verdict.decision === DECISION_NONE) { decision = DECISION_NONE; return; }
    if (verdict.decision === DECISION_REQUIRED) {
      decision = DECISION_REQUIRED;
      const why = verdict.reason === "" ? "the repository requires it" : verdict.reason;
      // The verdict names the debt; the toggle decides who acts on it. Off, the human still answers: the
      // repository is not overruled, only the automatic send is.
      if (await autoRunEnabled(ctx.cwd, env, exec)) {
        // The repository decided this run happens; asking would be a question whose answer is already known.
        declined.clear();
        persist();
        ctx.ui.notify(notice(`TSP required: ${why}`), "info");
        pi.sendUserMessage(TSP_PROMPT);
        return;
      }
      ctx.ui.notify(notice(`TSP required: ${why} · automatic run disabled (tsp feature enable auto-run)`), "warning");
    }
    const undecided = paths.filter((p) => !declined.has(p));
    if (undecided.length === 0) return;
    decision = DECISION_ASK;
    const owed = verdict.named ? verdict.debt : verdict.reason;
    if (owed === "") return;
    const answer = await ctx.ui.select(`${OFFER_QUESTION}\n${owed}`, [OFFER_YES, OFFER_NO]);
    if (answer === OFFER_YES) {
      declined.clear();
      persist();
      pi.sendUserMessage(TSP_PROMPT);
    } else if (answer === OFFER_NO) {
      paths.forEach((p) => declined.add(p));
      decision = undefined; // the human answered: nothing is owed until the next change
      persist();
    }
  };
  pi.on("agent_settled", async (event, ctx) => {
    try { await settle(event, ctx); } finally { if (ctx.hasUI) paint(ctx); }
  });
  return () => send(TSP_PROMPT, "tsp");
}

export default function (pi: ExtensionAPI, offer: OfferOptions = {}) {
  const startRun = registerOffer(pi, offer);
  const register = (name: "tsp" | "tpp") => pi.registerCommand(name, {
    description: `${name} run · check · feedback --summary · doctor [--mode auto|gentle] in this session`,
    handler: (args: string, ctx: ExtensionCommandContext) =>
      runTppCommand(args, {
        tools: pi.getAllTools(), cwd: ctx.cwd, env: process.env, hasUI: ctx.hasUI, exec: execTpp,
        notify: (message, type) => ctx.ui.notify(message, type),
        setStatus: (text) => ctx.ui.setStatus("tsp", text),
        startRun,
        show: (text) => ctx.ui.setWidget("tsp-output", text === undefined ? undefined : text.split("\n")),
      }),
  });
  register("tsp");
  register("tpp");
}
