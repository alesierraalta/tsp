const MINUTE = 60 * 1000;
const HOUR = 60 * MINUTE;
const DAY = 24 * HOUR;

export function toInstant(value) {
  if (value instanceof Date) return new Date(value.getTime());
  if (typeof value === 'number') return new Date(value);
  const text = String(value ?? '').trim();
  if (text === '') return undefined;
  const stamp = /^\d{4}-\d{2}-\d{2}$/.test(text) ? `${text}T00:00:00.000Z` : text;
  const parsed = new Date(stamp);
  return Number.isNaN(parsed.getTime()) ? undefined : parsed;
}

export function toIso(value) {
  return toInstant(value)?.toISOString();
}

export function nowIso(clock = () => new Date()) {
  return new Date(clock()).toISOString();
}

export function minuteBucket(value) {
  const instant = toInstant(value);
  if (!instant) return undefined;
  const time = Math.floor(instant.getTime() / MINUTE) * MINUTE;
  return new Date(time).toISOString();
}

export function hourBucket(value) {
  const instant = toInstant(value);
  if (!instant) return undefined;
  return new Date(Math.floor(instant.getTime() / HOUR) * HOUR).toISOString();
}

export function dayBucket(value) {
  return toInstant(value)?.toISOString().slice(0, 10);
}

export function bucketOf(value, size) {
  const instant = toInstant(value);
  if (!instant) return undefined;
  const unit = size === 'minute' ? MINUTE : size === 'hour' ? HOUR : DAY;
  return new Date(Math.floor(instant.getTime() / unit) * unit).toISOString();
}

export function addMilliseconds(value, ms) {
  const instant = toInstant(value) ?? new Date();
  return new Date(instant.getTime() + Math.trunc(ms));
}

export function addDays(value, days) {
  return addMilliseconds(value, Math.trunc(days) * DAY);
}

export function diffMilliseconds(from, to) {
  const start = toInstant(from);
  const end = toInstant(to);
  if (!start || !end) return 0;
  return end.getTime() - start.getTime();
}

export function diffSeconds(from, to) {
  return Math.round(diffMilliseconds(from, to) / 1000);
}

export function isWithin(value, from, to) {
  const instant = toInstant(value);
  if (!instant) return false;
  const start = toInstant(from)?.getTime();
  const end = toInstant(to)?.getTime();
  if (start !== undefined && instant.getTime() < start) return false;
  if (end !== undefined && instant.getTime() > end) return false;
  return true;
}

export function rangeOf(from, to, size = 'hour') {
  const out = [];
  const start = toInstant(from);
  const end = toInstant(to);
  if (!start || !end) return out;
  const unit = size === 'minute' ? MINUTE : size === 'day' ? DAY : HOUR;
  for (let time = start.getTime(); time <= end.getTime(); time += unit) {
    out.push(new Date(time).toISOString());
  }
  return out;
}

export function alignedWindow(value, size = 'hour', count = 1) {
  const bucket = bucketOf(value, size);
  if (!bucket) return undefined;
  const unit = size === 'minute' ? MINUTE : size === 'day' ? DAY : HOUR;
  return { from: bucket, to: new Date(toInstant(bucket).getTime() + unit * count).toISOString() };
}

export function parseWindow(text, fallback = '1h') {
  const match = /^(\d+)(m|h|d)$/.exec(String(text ?? '').trim());
  const unit = match ? match[2] : 'h';
  const count = match ? Number(match[1]) : Number.parseInt(fallback, 10);
  const ms = unit === 'm' ? MINUTE : unit === 'd' ? DAY : HOUR;
  return count * ms;
}

export function sortableInstant(value) {
  return toInstant(value)?.getTime() ?? 0;
}
