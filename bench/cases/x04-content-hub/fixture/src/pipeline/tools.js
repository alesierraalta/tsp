import { execFile } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

const TOOL_DIR = join(dirname(fileURLToPath(import.meta.url)), '..', '..', 'bin');

export const TOOLS = {
  markdown: { file: 'render.mjs', summary: 'render a markdown entry to html' },
  hash: { file: 'checksum.mjs', summary: 'print the checksum of a file' },
  resize: { file: 'resize.mjs', summary: 'copy an image at a fixed width' },
};

const MAX_ARG_LENGTH = 512;

export function assertArg(value) {
  const text = String(value ?? '');
  if (text.length > MAX_ARG_LENGTH) throw new TypeError(`argument is longer than ${MAX_ARG_LENGTH} characters`);
  if (text.includes('\u0000')) throw new TypeError('argument carries a null byte');
  if (/[\r\n]/.test(text)) throw new TypeError('argument carries a line break');
  return text;
}

export function assertArgs(args) {
  if (!Array.isArray(args)) throw new TypeError('tool arguments must be a list');
  return args.map((arg) => assertArg(arg));
}

export function toolPath(name) {
  const tool = TOOLS[name];
  if (!tool) throw new TypeError(`unknown tool ${name}`);
  return join(TOOL_DIR, tool.file);
}

export function toolCommand(name, args = []) {
  const clean = assertArgs(args);
  return { file: toolPath(name), args: clean };
}

export function runTool(name, args = [], options = {}) {
  const { file, args: clean } = toolCommand(name, args);
  return new Promise((resolveTool, rejectTool) => {
    execFile(process.execPath, [file, ...clean], { timeout: options.timeout ?? 20000, cwd: options.cwd }, (error, stdout, stderr) => {
      if (error) {
        const failure = new Error(`tool ${name} failed: ${String(stderr || error.message).trim()}`);
        failure.code = 'tool_failed';
        failure.status = 500;
        failure.toJSON = () => ({ error: 'tool_failed', message: failure.message });
        rejectTool(failure);
        return;
      }
      resolveTool({ tool: name, stdout: String(stdout), stderr: String(stderr) });
    });
  });
}

export function listTools() {
  return Object.entries(TOOLS).map(([name, tool]) => ({ name, summary: tool.summary }));
}

export function describeTool(name) {
  const tool = TOOLS[name];
  if (!tool) return undefined;
  return { name, file: tool.file, summary: tool.summary };
}

export async function runToolsSequential(steps, options = {}) {
  const results = [];
  for (const step of steps) {
    results.push(await runTool(step.tool, step.args ?? [], options));
  }
  return results;
}

export async function toolVersion(name) {
  const { stdout } = await runTool(name, ['--version']).catch(() => ({ stdout: 'unknown' }));
  return stdout.trim();
}

export function limit(args, max = 16) {
  if (args.length > max) throw new TypeError(`a tool takes at most ${max} arguments`);
  return args;
}

export function argumentsFor(tool, input = {}) {
  switch (tool) {
    case 'markdown':
      return [input.source ?? '', `--format=${input.format ?? 'fragment'}`];
    case 'hash':
      return [input.file ?? '']; 
    case 'resize':
      return [input.source ?? '', input.destination ?? '', `--width=${input.width ?? 480}`];
    default:
      throw new TypeError(`unknown tool ${tool}`);
  }
}

export function toolCatalogue() {
  return Object.entries(TOOLS).map(([name, tool]) => ({ name, file: tool.file, summary: tool.summary }));
}
