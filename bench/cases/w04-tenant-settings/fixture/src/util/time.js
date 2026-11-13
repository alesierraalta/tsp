export const SECOND = 1000;
export const MINUTE = 60 * SECOND;
export const HOUR = 60 * MINUTE;
export const DAY = 24 * HOUR;

export function toDate(value) {
  const date = value instanceof Date ? value : new Date(value);
  if (Number.isNaN(date.getTime())) {
    throw new TypeError(`not a date: ${value}`);
  }
  return date;
}

export function nowIso(clock = () => new Date()) {
  const value = typeof clock === 'function' ? clock() : clock;
  return toDate(value).toISOString();
}

export function addSeconds(iso, seconds) {
  return new Date(toDate(iso).getTime() + seconds * SECOND).toISOString();
}

export function addDays(iso, days) {
  return new Date(toDate(iso).getTime() + days * DAY).toISOString();
}

export function isExpired(iso, at = new Date()) {
  return toDate(iso).getTime() <= toDate(at).getTime();
}

export function dayKey(iso) {
  return toDate(iso).toISOString().slice(0, 10);
}

export function bucketStart(iso, minutes) {
  const step = minutes * MINUTE;
  const at = toDate(iso).getTime();
  return new Date(at - (at % step)).toISOString();
}

export function formatDuration(ms) {
  if (ms < SECOND) return `${Math.round(ms)}ms`;
  if (ms < MINUTE) return `${(ms / SECOND).toFixed(1)}s`;
  if (ms < HOUR) return `${Math.round(ms / MINUTE)}m`;
  return `${(ms / HOUR).toFixed(1)}h`;
}

export function ageSeconds(iso, at = new Date()) {
  return Math.max(0, Math.round((toDate(at).getTime() - toDate(iso).getTime()) / SECOND));
}
