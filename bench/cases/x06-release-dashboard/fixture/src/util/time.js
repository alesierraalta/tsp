const SECOND = 1000;
const MINUTE = 60 * SECOND;
const HOUR = 60 * MINUTE;
const DAY = 24 * HOUR;
const WEEK = 7 * DAY;

export const UNITS = { second: SECOND, minute: MINUTE, hour: HOUR, day: DAY, week: WEEK };

export function toInstant(value) {
  if (value instanceof Date) return new Date(value.getTime());
  if (typeof value === 'number') return new Date(value);
  const text = String(value ?? '').trim();
  if (text === '') return undefined;
  const stamped = /^\d{4}-\d{2}-\d{2}$/.test(text) ? `${text}T00:00:00.000Z` : text;
  const parsed = new Date(stamped);
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
  return instant === undefined ? undefined : new Date(Math.floor(instant.getTime() / MINUTE) * MINUTE).toISOString();
}

export function hourBucket(value) {
  const instant = toInstant(value);
  return instant === undefined ? undefined : new Date(Math.floor(instant.getTime() / HOUR) * HOUR).toISOString();
}

export function dayBucket(value) {
  return toInstant(value)?.toISOString().slice(0, 10);
}

export function monthKey(value) {
  return toInstant(value)?.toISOString().slice(0, 7);
}

export function weekKey(value) {
  const instant = toInstant(value);
  if (instant === undefined) return undefined;
  const start = new Date(instant.getTime() - ((instant.getUTCDay() + 6) % 7) * DAY);
  return dayBucket(start);
}

export function bucketOf(value, size = 'hour') {
  const instant = toInstant(value);
  if (instant === undefined) return undefined;
  const unit = UNITS[size] ?? HOUR;
  return new Date(Math.floor(instant.getTime() / unit) * unit).toISOString();
}

export function startOfDay(value) {
  const instant = toInstant(value);
  return instant === undefined ? undefined : new Date(`${dayBucket(instant)}T00:00:00.000Z`).toISOString();
}

export function addMilliseconds(value, milliseconds) {
  const instant = toInstant(value) ?? new Date();
  return new Date(instant.getTime() + Math.trunc(milliseconds));
}

export function addSeconds(value, seconds) {
  return addMilliseconds(value, Math.trunc(seconds) * SECOND);
}

export function addMinutes(value, minutes) {
  return addMilliseconds(value, Math.trunc(minutes) * MINUTE);
}

export function addHours(value, hours) {
  return addMilliseconds(value, Math.trunc(hours) * HOUR);
}

export function addDays(value, days) {
  return addMilliseconds(value, Math.trunc(days) * DAY);
}

export function diffMilliseconds(from, to) {
  const start = toInstant(from);
  const end = toInstant(to);
  if (start === undefined || end === undefined) return 0;
  return end.getTime() - start.getTime();
}

export function diffSeconds(from, to) {
  return Math.round(diffMilliseconds(from, to) / SECOND);
}

export function diffMinutes(from, to) {
  return Math.round(diffMilliseconds(from, to) / MINUTE);
}

export function diffHours(from, to) {
  return Math.round((diffMilliseconds(from, to) / HOUR) * 100) / 100;
}

export function isWithin(value, from, to) {
  const instant = toInstant(value);
  if (instant === undefined) return false;
  const start = toInstant(from);
  const end = toInstant(to);
  if (start !== undefined && instant.getTime() < start.getTime()) return false;
  if (end !== undefined && instant.getTime() > end.getTime()) return false;
  return true;
}

export function rangeOf(from, to, size = 'hour') {
  const start = toInstant(from);
  const end = toInstant(to);
  if (start === undefined || end === undefined) return [];
  const unit = UNITS[size] ?? HOUR;
  const out = [];
  for (let time = start.getTime(); time <= end.getTime(); time += unit) out.push(new Date(time).toISOString());
  return out;
}

export function parseDuration(text, fallback = '1h') {
  const match = /^(\d+)\s*(s|m|h|d|w)$/.exec(String(text ?? '').trim().toLowerCase());
  const source = match ? `${match[1]}${match[2]}` : String(fallback).trim().toLowerCase();
  const parsed = /^(\d+)(s|m|h|d|w)$/.exec(source);
  if (!parsed) return HOUR;
  const unit = { s: SECOND, m: MINUTE, h: HOUR, d: DAY, w: WEEK }[parsed[2]] ?? HOUR;
  return Number(parsed[1]) * unit;
}

export function formatDuration(milliseconds) {
  const value = Math.max(0, Math.trunc(Number(milliseconds) || 0));
  if (value < MINUTE) return `${Math.round(value / SECOND)}s`;
  if (value < HOUR) return `${Math.round(value / MINUTE)}m`;
  if (value < DAY) return `${Math.round((value / HOUR) * 10) / 10}h`;
  return `${Math.round((value / DAY) * 10) / 10}d`;
}

export function sortableInstant(value) {
  return toInstant(value)?.getTime() ?? 0;
}

export function alignedWindow(value, size = 'hour', count = 1) {
  const bucket = bucketOf(value, size);
  if (bucket === undefined) return undefined;
  const unit = UNITS[size] ?? HOUR;
  return { from: bucket, to: new Date(toInstant(bucket).getTime() + unit * Math.max(1, count)).toISOString() };
}

export function everyMinute(from, to) {
  return rangeOf(from, to, 'minute');
}

export function hoursBetween(from, to) {
  return round(diffHours(from, to));
}

export function round(value) {
  return Math.round((Number(value) || 0) * 100) / 100;
}

export function withinMinutes(value, minutes, clock = () => new Date()) {
  const instant = toInstant(value);
  if (instant === undefined) return false;
  const now = toInstant(clock());
  return Math.abs(now.getTime() - instant.getTime()) <= Math.max(0, Math.trunc(minutes)) * MINUTE;
}

export function describeSpan(from, to) {
  return { from: toIso(from), to: toIso(to), minutes: diffMinutes(from, to), hours: hoursBetween(from, to) };
}
