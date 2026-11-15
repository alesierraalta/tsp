export const SECOND = 1000;
export const MINUTE = 60 * SECOND;
export const HOUR = 60 * MINUTE;

export function toDate(value) {
  const date = value instanceof Date ? value : new Date(value);
  if (Number.isNaN(date.getTime())) throw new TypeError(`not a date: ${value}`);
  return date;
}

export function isoNow(clock = () => new Date()) {
  const value = typeof clock === 'function' ? clock() : clock;
  return toDate(value).toISOString();
}

export function addSeconds(iso, seconds) {
  return new Date(toDate(iso).getTime() + seconds * SECOND).toISOString();
}

export function isDue(iso, now = new Date()) {
  return toDate(iso).getTime() <= toDate(now).getTime();
}

export function ageSeconds(iso, now = new Date()) {
  return Math.max(0, Math.round((toDate(now).getTime() - toDate(iso).getTime()) / SECOND));
}

export function formatAge(iso, now = new Date()) {
  const seconds = ageSeconds(iso, now);
  if (seconds < 60) return `${seconds}s`;
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m`;
  return `${Math.floor(seconds / 3600)}h`;
}

export function bucketStart(iso, seconds) {
  const step = seconds * SECOND;
  const at = toDate(iso).getTime();
  return new Date(at - (at % step)).toISOString();
}

export function minuteKey(iso) {
  return toDate(iso).toISOString().slice(0, 16);
}

export function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}
