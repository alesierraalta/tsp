const DAY_MS = 24 * 60 * 60 * 1000;

export function isoNow(clock = () => new Date()) {
  return new Date(clock()).toISOString();
}

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
  const instant = toInstant(value);
  return instant ? instant.toISOString() : undefined;
}

export function dayOf(value) {
  const instant = toInstant(value);
  return instant ? instant.toISOString().slice(0, 10) : undefined;
}

export function addDays(value, days) {
  const instant = toInstant(value) ?? new Date();
  return new Date(instant.getTime() + Math.trunc(days) * DAY_MS);
}

export function addMinutes(value, minutes) {
  const instant = toInstant(value) ?? new Date();
  return new Date(instant.getTime() + Math.trunc(minutes) * 60 * 1000);
}

export function startOfDay(value) {
  const instant = toInstant(value) ?? new Date();
  return new Date(Date.UTC(instant.getUTCFullYear(), instant.getUTCMonth(), instant.getUTCDate()));
}

export function endOfDay(value) {
  return new Date(startOfDay(value).getTime() + DAY_MS - 1);
}

export function isPast(value, clock = () => new Date()) {
  const instant = toInstant(value);
  if (!instant) return false;
  return instant.getTime() <= new Date(clock()).getTime();
}

export function isFuture(value, clock = () => new Date()) {
  const instant = toInstant(value);
  if (!instant) return false;
  return instant.getTime() > new Date(clock()).getTime();
}

export function durationMinutes(from, to) {
  const start = toInstant(from);
  const end = toInstant(to);
  if (!start || !end) return 0;
  return Math.round((end.getTime() - start.getTime()) / 60000);
}

export function nextRun(previous, everyMinutes, clock = () => new Date()) {
  const base = toInstant(previous) ?? new Date(clock());
  return new Date(base.getTime() + Math.max(1, Math.trunc(everyMinutes)) * 60000);
}

export function relative(instant, clock = () => new Date()) {
  const when = toInstant(instant);
  if (!when) return 'unknown';
  const minutes = Math.round((when.getTime() - new Date(clock()).getTime()) / 60000);
  if (minutes === 0) return 'now';
  const past = minutes < 0;
  const size = Math.abs(minutes);
  const label = size < 60 ? `${size}m` : size < 1440 ? `${Math.round(size / 60)}h` : `${Math.round(size / 1440)}d`;
  return past ? `${label} ago` : `in ${label}`;
}

export function compareInstants(left, right) {
  const a = toInstant(left);
  const b = toInstant(right);
  if (!a && !b) return 0;
  if (!a) return -1;
  if (!b) return 1;
  return a.getTime() - b.getTime();
}

export function isoWeek(value) {
  const instant = toInstant(value) ?? new Date();
  const target = new Date(Date.UTC(instant.getUTCFullYear(), instant.getUTCMonth(), instant.getUTCDate()));
  const day = target.getUTCDay() || 7;
  target.setUTCDate(target.getUTCDate() + 4 - day);
  const yearStart = new Date(Date.UTC(target.getUTCFullYear(), 0, 1));
  const week = Math.ceil(((target.getTime() - yearStart.getTime()) / 86400000 + 1) / 7);
  return { year: target.getUTCFullYear(), week };
}

export function monthWindow(value) {
  const instant = toInstant(value) ?? new Date();
  const start = new Date(Date.UTC(instant.getUTCFullYear(), instant.getUTCMonth(), 1));
  const end = new Date(Date.UTC(instant.getUTCFullYear(), instant.getUTCMonth() + 1, 0, 23, 59, 59, 999));
  return { from: start.toISOString(), to: end.toISOString() };
}

export function daysBetween(from, to) {
  const start = startOfDay(from);
  const end = startOfDay(to);
  return Math.round((end.getTime() - start.getTime()) / DAY_MS);
}

export function bucketByDay(values) {
  const out = new Map();
  for (const value of values ?? []) {
    const day = dayOf(value);
    if (!day) continue;
    out.set(day, (out.get(day) ?? 0) + 1);
  }
  return [...out.entries()].sort((left, right) => (left[0] < right[0] ? -1 : 1)).map(([day, count]) => ({ day, count }));
}

export function parseDuration(text) {
  const match = /^(\d+)\s*(m|h|d|w)$/i.exec(String(text ?? '').trim());
  if (!match) return undefined;
  const amount = Number(match[1]);
  const unit = match[2].toLowerCase();
  const minutes = unit === 'm' ? amount : unit === 'h' ? amount * 60 : unit === 'd' ? amount * 1440 : amount * 10080;
  return minutes;
}

export function formatDuration(minutes) {
  const total = Math.max(0, Math.trunc(Number(minutes) || 0));
  if (total < 60) return `${total}m`;
  if (total < 1440) return `${Math.floor(total / 60)}h ${total % 60}m`;
  return `${Math.floor(total / 1440)}d ${Math.floor((total % 1440) / 60)}h`;
}

export function isSameDay(left, right) {
  const a = dayOf(left);
  const b = dayOf(right);
  return a !== undefined && a === b;
}

export function clampInstant(value, from, to) {
  const when = toInstant(value);
  if (!when) return undefined;
  const lower = toInstant(from)?.getTime();
  const upper = toInstant(to)?.getTime();
  if (lower !== undefined && when.getTime() < lower) return new Date(lower);
  if (upper !== undefined && when.getTime() > upper) return new Date(upper);
  return when;
}
