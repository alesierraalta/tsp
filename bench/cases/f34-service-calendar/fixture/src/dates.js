export const DAY = 86400000;

export function instant(value) {
  const ms = typeof value === 'number' ? value : Date.parse(value);
  if (!Number.isSafeInteger(ms)) throw new TypeError('invalid instant');
  return ms;
}

export function dayKey(value) {
  return new Date(instant(value)).toISOString().slice(0, 10);
}

export function monthKey(value) {
  return dayKey(value).slice(0, 7);
}

export function midnight(value) {
  return instant(`${dayKey(value)}T00:00:00.000Z`);
}

export function addDays(value, count) {
  if (!Number.isInteger(count)) throw new TypeError('day count');
  return instant(value) + count * DAY;
}

export function monthStart(value) {
  return instant(`${monthKey(value)}-01T00:00:00.000Z`);
}

export function daysInMonth(year, month) {
  if (!Number.isInteger(year) || year < 1970 || year > 9999) throw new TypeError('year');
  if (!Number.isInteger(month) || month < 1 || month > 12) throw new TypeError('month');
  return new Date(Date.UTC(year, month, 0)).getUTCDate();
}

export function monthParts(value) {
  const date = new Date(instant(value));
  return { year: date.getUTCFullYear(), month: date.getUTCMonth() + 1 };
}

export function compareDays(a, b) {
  return Math.sign(midnight(a) - midnight(b));
}

export function range(start, count) {
  if (!Number.isInteger(count) || count < 0 || count > 366) throw new TypeError('count');
  return Array.from({ length: count }, (_, i) => dayKey(addDays(start, i)));
}

export function format(value) {
  return new Date(instant(value)).toISOString();
}
