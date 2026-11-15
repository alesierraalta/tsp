import { exec } from 'node:child_process';
import { hookFailed } from '../util/errors.js';
import { runHook } from '../hooks/runner.js';

export const IMPORT_EVENT = 'import.finished';

export function hookCommand(hook, payloadName) {
  return `"${process.execPath}" "${hook.script}" ${payloadName}`;
}

export function runPostImportHook(registry, payloadName, options = {}) {
  const hook = registry.forEvent(IMPORT_EVENT);
  if (!hook) {
    return Promise.resolve({ skipped: true, reason: `no ${IMPORT_EVENT} hook` });
  }
  const timeoutMs = options.timeoutMs ?? hook.timeoutMs ?? 10000;
  return new Promise((resolve, reject) => {
    exec(
      hookCommand(hook, payloadName),
      { timeout: timeoutMs, env: { ...process.env, ...(options.env ?? {}) } },
      (error, stdout, stderr) => {
        if (error) {
          reject(
            hookFailed(`hook ${hook.name} failed: ${error.message}`, {
              stdout: String(stdout).trim(),
              stderr: String(stderr).trim(),
            }),
          );
          return;
        }
        resolve({ code: 0, stdout: String(stdout).trim(), stderr: String(stderr).trim() });
      },
    );
  });
}

export function hookPlan(registry) {
  return registry
    .allForEvent(IMPORT_EVENT)
    .map((hook) => ({ name: hook.name, command: hookCommand(hook, '<payload>') }));
}

export function postImportSummary(registry, payloadName) {
  const hook = registry.forEvent(IMPORT_EVENT);
  if (!hook) return 'no hook';
  return `${hook.name} <- ${payloadName}`;
}

export const safeRunHook = runHook;
