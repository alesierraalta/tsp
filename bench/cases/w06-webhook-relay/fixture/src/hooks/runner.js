import { spawn } from 'node:child_process';
import { isAbsolute } from 'node:path';
import { hookFailed } from '../util/errors.js';

export const HOOK_ARGUMENT = /^[A-Za-z0-9][A-Za-z0-9._-]{0,119}$/;

export function assertHookArgument(value) {
  const text = String(value ?? '');
  if (!HOOK_ARGUMENT.test(text)) {
    throw hookFailed(`unsafe hook argument: ${text}`);
  }
  return text;
}

export function assertScriptPath(script) {
  const text = String(script ?? '');
  if (!isAbsolute(text)) throw hookFailed(`hook script must be an absolute path: ${text}`);
  if (!/\.(mjs|js|cjs)$/.test(text)) throw hookFailed(`hook script must be a module: ${text}`);
  return text;
}

export async function runHook(script, args = [], options = {}) {
  const program = assertScriptPath(script);
  const clean = args.map((arg) => assertHookArgument(arg));
  const timeoutMs = options.timeoutMs ?? 10000;
  return new Promise((resolve, reject) => {
    const child = spawn(process.execPath, [program, ...clean], {
      cwd: options.cwd,
      env: { ...process.env, ...(options.env ?? {}) },
      stdio: ['ignore', 'pipe', 'pipe'],
      shell: false,
    });
    let stdout = '';
    let stderr = '';
    let settled = false;
    const timer = setTimeout(() => {
      if (settled) return;
      settled = true;
      child.kill('SIGKILL');
      reject(hookFailed(`hook ${program} timed out after ${timeoutMs}ms`));
    }, timeoutMs);

    child.stdout.on('data', (chunk) => {
      stdout += chunk;
    });
    child.stderr.on('data', (chunk) => {
      stderr += chunk;
    });
    child.on('error', (error) => {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      reject(hookFailed(`hook ${program} could not start: ${error.message}`));
    });
    child.on('close', (code) => {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      if (code !== 0) {
        reject(hookFailed(`hook ${program} exited ${code}`, { stdout: stdout.trim(), stderr: stderr.trim() }));
        return;
      }
      resolve({ code, stdout: stdout.trim(), stderr: stderr.trim() });
    });
  });
}

export function hookSummary(result) {
  if (result.skipped) return `skipped: ${result.reason}`;
  return `exit ${result.code} ${result.stdout}`.trim();
}
