export const DAY = 24 * 60 * 60 * 1000;

export function toDate(value) {
  const date = value instanceof Date ? value : new Date(value);
  if (Number.isNaN(date.getTime())) throw new TypeError(`not a date: ${value}`);
  return date;
}

export function isoDay(value = new Date()) {
  return toDate(value).toISOString().slice(0, 10);
}

export function isDateString(value) {
  if (typeof value !== 'string') return false;
  if (!/^\d{4}-\d{2}-\d{2}$/.test(value)) return false;
  const parsed = new Date(`${value}T00:00:00Z`);
  return !Number.isNaN(parsed.getTime()) && isoDay(parsed) === value;
}

export function monthKey(value) {
  return toDate(value).toISOString().slice(0, 7);
}

export function addDays(value, days) {
  return new Date(toDate(value).getTime() + days * DAY).toISOString();
}

export function daysBetween(left, right) {
  return Math.round((toDate(right).getTime() - toDate(left).getTime()) / DAY);
}

export function quarterOf(value) {
  const date = toDate(value);
  return Math.floor(date.getUTCMonth() / 3) + 1;
}

export function isoWeek(value) {
  const date = toDate(value);
  const target = new Date(Date.UTC(date.getUTCFullYear(), date.getUTCMonth(), date.getUTCDate()));
  const dayNumber = (target.getUTCDay() + 6) % 7;
  target.setUTCDate(target.getUTCDate() - dayNumber + 3);
  const firstThursday = new Date(Date.UTC(target.getUTCFullYear(), 0, 4));
  const offset = (firstThursday.getUTCDay() + 6) % 7;
  firstThursday.setUTCDate(firstThursday.getUTCDate() - offset + 3);
  return 1 + Math.round((target.getTime() - firstThursday.getTime()) / (7 * DAY));
}

export function formatDayList(days) {
  return [...new Set(days)].sort().join(', ');
}
