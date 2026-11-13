import { toInstant, toIso } from '../util/time.js';

const DAYS = ['monday', 'tuesday', 'wednesday', 'thursday', 'friday', 'saturday', 'sunday'];

export function normalizeWindow(input = {}) {
  const from = toIso(input.from);
  const to = toIso(input.to);
  if (!from || !to) throw new TypeError('a window needs from and to instants');
  if (toInstant(from).getTime() > toInstant(to).getTime()) {
    throw new TypeError('a window cannot end before it starts');
  }
  return { from, to };
}

export function windowContains(window, instant) {
  const when = toInstant(instant)?.getTime();
  if (when === undefined) return false;
  return when >= toInstant(window.from).getTime() && when <= toInstant(window.to).getTime();
}

export function nextWindow(windows, instant) {
  const when = toInstant(instant)?.getTime() ?? Date.now();
  return windows
    .map((window) => ({ ...window, at: toInstant(window.from).getTime() }))
    .filter((window) => window.at > when)
    .sort((left, right) => left.at - right.at)[0];
}

export function weekWindow(instant, startDay = 'monday') {
  const base = toInstant(instant) ?? new Date();
  const index = DAYS.indexOf(startDay);
  if (index < 0) throw new TypeError(`unknown day ${startDay}`);
  const day = base.getUTCDay();
  const offset = (day - (index === 6 ? 0 : index + 1) + 7) % 7;
  const start = new Date(Date.UTC(base.getUTCFullYear(), base.getUTCMonth(), base.getUTCDate() - offset));
  const end = new Date(start.getTime() + 7 * 24 * 60 * 60 * 1000 - 1);
  return { from: start.toISOString(), to: end.toISOString() };
}

export function dayWindow(instant) {
  const base = toInstant(instant) ?? new Date();
  const start = new Date(Date.UTC(base.getUTCFullYear(), base.getUTCMonth(), base.getUTCDate()));
  return { from: start.toISOString(), to: new Date(start.getTime() + 24 * 60 * 60 * 1000 - 1).toISOString() };
}

export function windowHours(window) {
  const from = toInstant(window.from);
  const to = toInstant(window.to);
  if (!from || !to) return 0;
  return Math.round(((to.getTime() - from.getTime()) / 3_600_000) * 100) / 100;
}

export function windowsOverlap(left, right) {
  return toInstant(left.from).getTime() <= toInstant(right.to).getTime()
    && toInstant(right.from).getTime() <= toInstant(left.to).getTime();
}

export function mergeWindows(windows) {
  const sorted = [...windows]
    .map((window) => normalizeWindow(window))
    .sort((left, right) => toInstant(left.from).getTime() - toInstant(right.from).getTime());
  const out = [];
  for (const window of sorted) {
    const last = out[out.length - 1];
    if (last && windowsOverlap(last, window)) {
      if (toInstant(window.to).getTime() > toInstant(last.to).getTime()) last.to = window.to;
      continue;
    }
    out.push({ ...window });
  }
  return out;
}

export function availableWindows(schedule, instant) {
  const when = instant ?? new Date().toISOString();
  return (schedule ?? []).filter((window) => !windowContains(window, when));
}
