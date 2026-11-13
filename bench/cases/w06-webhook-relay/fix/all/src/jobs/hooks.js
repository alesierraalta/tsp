import { assertHookArgument, runHook } from '../hooks/runner.js';

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
  return runHook(hook.script, [assertHookArgument(payloadName)], {
    timeoutMs,
    env: options.env,
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
