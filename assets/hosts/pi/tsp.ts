// tsp for Pi: the shared tsp CLI as a native /tsp command, loadable without an installer:
//   pi --extension /path/to/assets/hosts/pi/tsp.ts
//
// Detection reads this session's registered tools (pi.getAllTools()), never PATH: exact
// Gentle-owned tool names or whole gentle-pi/gentle-ai source path segments. When Gentle is
// observed, the spawned `tsp doctor` child gets the process-scoped observation through
// child_process.execFile's env — no process.env mutation, no file. That observation is session
// UX evidence for mode selection, not authentication or security authority. Gentle review
// authority is never called; UI writes are guarded by ctx.hasUI.

import { execFile } from "node:child_process";
import { existsSync } from "node:fs";
import { join } from "node:path";
import type { ExtensionAPI, ExtensionCommandContext } from "@earendil-works/pi-coding-agent";

/** Protocol with internal/mode (Go); mode.VerifiedGentleSignal accepts only this exact pair. */
export const GENTLE_OBSERVATION_ENV = "TPP_GENTLE_OBSERVATION";
export const GENTLE_OBSERVATION_VALUE = "pi-session-gentle-active";

export interface RegisteredTool {
  name: string;
  sourceInfo?: { path?: string };
}

export type TppDispatch =
  | { kind: "check"; rest: string[] } | { kind: "feedback-summary" }
  | { kind: "doctor"; mode: "auto" | "gentle" } | { kind: "help"; reason: string };

// The Gentle-owned tools registered by gentle-pi, by exact name — not any gentle_* prefix.
const GENTLE_TOOLS = new Set(["gentle_odd_phase", "gentle_review", "gentle_review_capture", "gentle_review_capture_group", "gentle_review_scope"]);
// Gentle-owned directories, matched as whole path segments only — never as substrings.
const GENTLE_DIRS = new Set(["gentle-pi", "gentle-ai"]);

/** True when Gentle is actually registered in this session — no PATH lookup, no version probe. */
export function observesGentleSession(tools: readonly RegisteredTool[]): boolean {
  return tools.some((t) => GENTLE_TOOLS.has(t.name) || (t.sourceInfo?.path ?? "").split(/[\\/]+/).some((s) => GENTLE_DIRS.has(s)));
}

const HELP = "Usage: /tsp check [--path <plan.md>] [--suite \"<test command>\"] [--no-test-needed \"<reason>\"] · /tsp feedback --summary · /tsp doctor [--mode auto|gentle]";

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
    case "help": return [];
  }
}

/** Child env for one tpp process: the observation rides only an observed doctor child; base is copied, never mutated. */
export function childEnv(base: Readonly<Record<string, string | undefined>>, d: TppDispatch, gentleObserved: boolean): Record<string, string | undefined> {
  const env: Record<string, string | undefined> = { ...base };
  if (gentleObserved && d.kind === "doctor") env[GENTLE_OBSERVATION_ENV] = GENTLE_OBSERVATION_VALUE;
  return env;
}

type ExecResult = { stdout: string; stderr: string; code: number };
type ExecOpts = { cwd: string; env: Record<string, string | undefined> };
export type ExecTpp = (argv: readonly string[], opts: ExecOpts) => Promise<ExecResult>;

/** One tpp child without a shell; a non-zero exit is data — only a binary that never started rejects. */
export function execTpp(argv: readonly string[], opts: ExecOpts): Promise<ExecResult> {
  return new Promise((resolve, reject) => {
    execFile("tsp", [...argv], { cwd: opts.cwd, env: opts.env as NodeJS.ProcessEnv, encoding: "utf8" }, (error, stdout, stderr) => {
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
    deps.notify(notice(output || `tsp ${label}: ${ok ? "ok" : `exited ${outcome.code}`}`), ok ? "info" : "error");
    const suffix = observed && d.kind === "doctor" ? " · gentle session observed" : "";
    deps.setStatus?.(`tsp ${label}: ${ok ? "ok" : `exit ${outcome.code}`}${suffix}`);
  }
}

// The TSP offer. When a run ends after the agent edited source code, and the repository still owes that change
// a test, Pi asks in the chat whether to run TSP; "Sí" sends "haz el TSP" as if the user typed it, which invokes
// the test-strategy skill. Only edit and write results are seen, so a change made through a shell command is not
// noticed. `tsp check` is the criterion: the edit list only says source was touched, while the check says whether
// TSP is owed and names what is missing, so a check that finds nothing owed or cannot run asks nothing.
// When a TSP run ends, the tests it left failing on HEAD's code are recorded as findings and the review of every
// finding is shown to the user; the agent is never told about it. A delegated child session
// (GENTLE_PI_AGENTS_CHILD=1) has nobody to answer, so it gets neither.
export const OFFER_QUESTION = "¿Quieres hacer TSP?";
export const OFFER_YES = "Sí";
export const OFFER_NO = "No";
export const TSP_PROMPT = "haz el TSP";

const SOURCE_EXT = /\.(go|ts|tsx|js|jsx|mjs|cjs|py|rs|java|kt|rb|php|cs|c|cc|cpp|h|hpp|swift|scala|sh)$/;
const TEST_PATH = /(^|\/)(tests?|__tests__|testdata|e2e|spec)\/|_test\.(go|py)$|\.(test|spec)\.[^/]+$|(^|\/)test_[^/]+\.py$/;

/** True for a source file the agent may have changed behaviour in; tests, docs and config are not. */
export function isSourcePath(path: string): boolean {
  const p = path.replace(/\\/g, "/");
  return SOURCE_EXT.test(p) && !TEST_PATH.test(p);
}

/** A run the user started to do TSP: its own edits never trigger another offer. */
const isTspPrompt = (prompt: string | undefined): boolean => /\b(tsp|test[- ]strategy)\b/i.test(prompt ?? "");

/**
 * What the source this run touched still owes a test for, or "" when it owes nothing: the offer's whole criterion.
 * The edit list only says the agent touched source; scoped `tsp check --paths` answers for those paths alone, so a
 * plan target still open from earlier work is never mistaken for a debt this run created, and a run that changed no
 * production source is never asked about. Exit 0 owes nothing and exit 1 owes, whatever the report's wording: its
 * first line that is not bookkeeping is the reason. Any other exit is the check failing, and a reply with no line at
 * all is the check unable to speak, so neither yields a criterion and the offer stays silent.
 */
async function owedReason(cwd: string, env: Readonly<Record<string, string | undefined>>, exec: ExecTpp, paths: readonly string[]): Promise<string> {
  let result: ExecResult;
  try {
    result = await exec(["check", "--cwd", cwd, "--paths", paths.join(",")], { cwd, env: { ...env } });
  } catch {
    return "";
  }
  if (result.code !== 1) return "";
  const reply = `${result.stdout}\n${result.stderr}`.split("\n").map((l) => l.trim()).filter((l) => l !== "");
  const owed = reply.filter((l) => !META_LINE.some((meta) => meta.test(l))).filter(hasVisibleText).map(boundReason);
  return owed[0] ?? "";
}

/**
 * A line is a reason only when it carries something that paints: a letter, a digit or punctuation. A line of controls,
 * formatting characters or bare glyph fillers renders as nothing, and would ask the question with a blank line while
 * hiding the debt line printed after it. Listing the invisible categories instead always leaves one out, so the test
 * asks the positive question. It reads the line as the check printed it, before the cut, whose ellipsis would make an
 * invisible line look visible. The Hangul fillers are letters that paint nothing, so they do not count as paint.
 */
function hasVisibleText(line: string): boolean {
  return /[\p{L}\p{N}\p{P}]/u.test(line.replace(/[\u115F\u1160\u3164\uFFA0]/g, ""));
}

/**
 * The reason as it may appear in a UI line: the line can quote plan-file text verbatim, so control characters never
 * survive it and it is capped at MAX_REASON characters, an ellipsis marking the cut. The cap sits above the longest
 * line the check itself prints, so its own wording always reaches the human whole; only plan text runs into it.
 * The cut counts characters, not their UTF-16 halves, so it never leaves a lone surrogate for the UI to render.
 */
const MAX_REASON = 300;
function boundReason(line: string): string {
  const clean = line.replace(/[\u0000-\u001f\u007f-\u009f]/g, "");
  const chars = [...clean];
  return chars.length > MAX_REASON ? chars.slice(0, MAX_REASON - 1).join("") + "…" : clean;
}

/**
 * The check's bookkeeping lines: a warning about its own evidence, the count of files that moved, and the aside that
 * labels plan data. They say why the check can speak and what the agent or the plan did, never what is owed, so the
 * offer never asks a question about them. The parenthesized one is only that exact marker: any other line fully in
 * parentheses can be the debt itself (a pending plan target) and must survive to the question.
 */
const META_LINE = [/^warning: /, /^\d+ production source file\(s\) changed: /, /^\(the names below are read from the plan file\b/];

export interface OfferOptions {
  optedOut?: (cwd: string) => boolean; // the repo's .no-testing-gate opt-out, shared with the Stop gate
  env?: Readonly<Record<string, string | undefined>>; // defaults to process.env
  exec?: ExecTpp; // defaults to the tsp binary
}

type Notify = (message: string, type?: "info" | "warning" | "error") => void;
const REVIEW_TYPE = { 0: "info", 1: "warning" } as const;

/** Records the TSP run's witnesses, then shows the review; a record that cannot run stops before check. */
export async function reviewFindings(cwd: string, env: Readonly<Record<string, string | undefined>>, exec: ExecTpp, notify: Notify): Promise<void> {
  const run = (sub: string) => exec(["findings", sub, "--cwd", cwd], { cwd, env: { ...env } });
  const shown = (r: ExecResult) => notice(r.stdout.trim() || r.stderr.trim() || `tsp findings: exited ${r.code}`);
  try {
    const recorded = await run("record");
    if (recorded.code !== 0 && recorded.code !== 1) return notify(shown(recorded), "error");
    const review = await run("check");
    notify(shown(review), REVIEW_TYPE[review.code as 0 | 1] ?? "error");
  } catch (error) {
    notify(notice(`tsp: ${error instanceof Error ? error.message : String(error)}`), "error");
  }
}

function registerOffer(pi: ExtensionAPI, opts: OfferOptions): void {
  const optedOut = opts.optedOut ?? ((cwd: string) => existsSync(join(cwd, ".no-testing-gate")));
  const env = opts.env ?? process.env;
  const exec = opts.exec ?? execTpp;
  const child = env.GENTLE_PI_AGENTS_CHILD === "1";
  const edited = new Set<string>(); // source paths changed in the current run
  const declined = new Set<string>(); // paths the user already said No to
  let tspRun = false;
  pi.on("before_agent_start", (event) => {
    edited.clear();
    tspRun = event.messageType === "user" && isTspPrompt(event.prompt);
  });
  pi.on("tool_result", (event) => {
    const path = (event.input as { path?: unknown }).path;
    if ((event.toolName === "edit" || event.toolName === "write") && !event.isError && typeof path === "string" && isSourcePath(path)) edited.add(path);
  });
  pi.on("agent_settled", async (event, ctx) => {
    const fresh = [...edited].filter((p) => !declined.has(p));
    edited.clear();
    if (event.aborted || child || !ctx.hasUI || optedOut(ctx.cwd)) return;
    if (tspRun) return reviewFindings(ctx.cwd, env, exec, (message, type) => ctx.ui.notify(message, type));
    if (fresh.length === 0) return;
    const owed = await owedReason(ctx.cwd, env, exec, fresh);
    if (owed === "") return;
    const answer = await ctx.ui.select(`${OFFER_QUESTION}\n${owed}`, [OFFER_YES, OFFER_NO]);
    if (answer === OFFER_YES) {
      declined.clear();
      pi.sendUserMessage(TSP_PROMPT);
    } else if (answer === OFFER_NO) {
      fresh.forEach((p) => declined.add(p));
    }
  });
}

export default function (pi: ExtensionAPI, offer: OfferOptions = {}) {
  registerOffer(pi, offer);
  const register = (name: "tsp" | "tpp") => pi.registerCommand(name, {
    description: `${name} check · feedback --summary · doctor [--mode auto|gentle] in this session`,
    handler: (args: string, ctx: ExtensionCommandContext) =>
      runTppCommand(args, {
        tools: pi.getAllTools(), cwd: ctx.cwd, env: process.env, hasUI: ctx.hasUI, exec: execTpp,
        notify: (message, type) => ctx.ui.notify(message, type),
        setStatus: (text) => ctx.ui.setStatus("tsp", text),
      }),
  });
  register("tsp");
  register("tpp");
}
