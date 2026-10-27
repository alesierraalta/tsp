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
// a test, Pi runs TSP on its own when the repository says so and asks in the chat when the repository left the
// decision open; "Sí" sends "haz el TSP" as if the user typed it, which invokes the test-strategy skill. Only edit
// and write results are seen, so a change made through a shell command is not noticed. `tsp check --json` is the
// criterion and the policy: it answers for the paths this run touched AND for the commit the turn started from,
// because a turn that commits its work unit leaves a clean tree the check alone would read as nothing changed.
// Its verdict names the decision — required means the repository declared the run, ask means it left that to the
// human, none or an error means the offer stays silent and Pi never asks a question whose answer is already known.
// When a TSP run ends, the tests it left failing on HEAD's code are recorded as findings and the review of every
// finding is shown to the user; the agent is never told about it. Then the agent is asked to record its feedback on
// that run on the template `tsp feedback --template` prints, so each run leaves a report later runs can be tuned on.
// That request is a run of its own: it reviews no findings, asks no TSP question and requests no more feedback.
// A delegated child session (GENTLE_PI_AGENTS_CHILD=1) has nobody to answer, so it gets none of them.
export const OFFER_QUESTION = "¿Quieres hacer TSP?";
export const OFFER_YES = "Sí";
export const OFFER_NO = "No";
export const TSP_PROMPT = "haz el TSP";
export const FEEDBACK_LEAD = "The TSP run has ended. Record honest feedback on it.";

const SOURCE_EXT = /\.(go|ts|tsx|js|jsx|mjs|cjs|py|rs|java|kt|rb|php|cs|c|cc|cpp|h|hpp|swift|scala|sh)$/;
const TEST_PATH = /(^|\/)(tests?|__tests__|testdata|e2e|spec)\/|_test\.(go|py)$|\.(test|spec)\.[^/]+$|(^|\/)test_[^/]+\.py$/;

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

/**
 * What the check decided about the source this run touched, or undefined when it decided nothing: the offer's whole
 * criterion and policy in one object. The edit list only says the agent touched source; the check answers for those
 * paths alone, so a plan target still open from earlier work is never mistaken for a debt this run created, and it
 * reads the commit this turn started from as well, so a turn that committed its work unit is still answered for.
 * `decision` is required, ask or none; anything else — an error verdict, a reply that is not one object, a child that
 * cannot run — yields no criterion and the offer stays silent.
 */
async function checkVerdict(cwd: string, env: Readonly<Record<string, string | undefined>>, exec: ExecTpp, paths: readonly string[], since: string): Promise<Verdict | undefined> {
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
    return undefined;
  }
  const { decision, debt, reason } = (reply ?? {}) as { decision?: unknown; debt?: unknown; reason?: unknown };
  if (decision !== DECISION_NONE && decision !== DECISION_ASK && decision !== DECISION_REQUIRED) return undefined;
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

export interface Verdict {
  decision: typeof DECISION_NONE | typeof DECISION_ASK | typeof DECISION_REQUIRED;
  named: boolean; // the verdict named a debt at all
  debt: string; // what this change owes, ready to show
  reason: string; // what decided the level
}
const DECISION_NONE = "none";
const DECISION_ASK = "ask";
const DECISION_REQUIRED = "required";

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
}

/** The commit this turn starts from, or "" when it cannot be read: a repository with no commit yet owes nothing. */
export function headAt(cwd: string): string {
  try {
    return execFileSync("git", ["rev-parse", "HEAD"], { cwd, encoding: "utf8", stdio: ["ignore", "pipe", "ignore"] }).trim();
  } catch {
    return "";
  }
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

function registerOffer(pi: ExtensionAPI, opts: OfferOptions): void {
  const optedOut = opts.optedOut ?? ((cwd: string) => existsSync(join(cwd, ".no-testing-gate")));
  const env = opts.env ?? process.env;
  const exec = opts.exec ?? execTpp;
  const head = opts.head ?? headAt;
  const child = env.GENTLE_PI_AGENTS_CHILD === "1";
  const edited = new Set<string>(); // source paths changed in the current run
  const declined = new Set<string>(); // paths the user already said No to
  let turnHead = ""; // the commit this turn started from: what --since reads the work unit against
  let tspRun = false;
  let feedbackRun = false;
  pi.on("before_agent_start", (event, ctx) => {
    edited.clear();
    turnHead = head(ctx.cwd);
    feedbackRun = event.messageType === "user" && (event.prompt ?? "").startsWith(FEEDBACK_LEAD);
    tspRun = !feedbackRun && event.messageType === "user" && isTspRequest(event.prompt);
  });
  pi.on("tool_result", (event, ctx) => {
    if (!feedbackRun && !event.isError && loadsTspSkill(event.toolName, event.input, ctx.cwd)) tspRun = true;
    const path = (event.input as { path?: unknown }).path;
    if ((event.toolName === "edit" || event.toolName === "write") && !event.isError && typeof path === "string" && isSourcePath(path)) edited.add(path);
  });
  pi.on("agent_settled", async (event, ctx) => {
    const fresh = [...edited].filter((p) => !declined.has(p));
    edited.clear();
    if (event.aborted || child || !ctx.hasUI || optedOut(ctx.cwd) || feedbackRun) return;
    if (tspRun) {
      const notify: Notify = (message, type) => ctx.ui.notify(message, type);
      await reviewFindings(ctx.cwd, env, exec, notify);
      const request = await feedbackRequest(ctx.cwd, env, exec, notify);
      if (request !== "") pi.sendUserMessage(request);
      return;
    }
    if (fresh.length === 0) return;
    const verdict = await checkVerdict(ctx.cwd, env, exec, fresh, turnHead);
    if (verdict === undefined || verdict.decision === DECISION_NONE) return;
    if (verdict.decision === DECISION_REQUIRED) {
      // The repository decided this run happens; asking would be a question whose answer is already known.
      declined.clear();
      const why = verdict.reason === "" ? "the repository requires it" : verdict.reason;
      ctx.ui.notify(notice(`TSP required: ${why}`), "info");
      pi.sendUserMessage(TSP_PROMPT);
      return;
    }
    const owed = verdict.named ? verdict.debt : verdict.reason;
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
