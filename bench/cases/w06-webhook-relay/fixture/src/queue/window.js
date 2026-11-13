const MINUTES_PER_DAY = 24 * 60;

export const WEEKDAYS = ['mon', 'tue', 'wed', 'thu', 'fri', 'sat', 'sun'];

export function parseClock(text) {
  const match = /^([01]\d|2[0-3]):([0-5]\d)$/.exec(String(text ?? '').trim());
  if (!match) throw new TypeError(`not a clock time: ${text}`);
  return Number(match[1]) * 60 + Number(match[2]);
}

export function formatClock(minutes) {
  const value = ((Number(minutes) % MINUTES_PER_DAY) + MINUTES_PER_DAY) % MINUTES_PER_DAY;
  return `${String(Math.floor(value / 60)).padStart(2, '0')}:${String(value % 60).padStart(2, '0')}`;
}

export function parseWindow(text) {
  const raw = String(text ?? '').trim();
  const [start, end] = raw.split('-');
  if (!start || !end) throw new TypeError(`not a window: ${text}`);
  const from = parseClock(start);
  const to = parseClock(end);
  return {
    text: raw,
    from,
    to,
    overnight: to <= from,
    minutes: to <= from ? MINUTES_PER_DAY - from + to : to - from,
  };
}

export function minuteOfDay(date) {
  const at = date instanceof Date ? date : new Date(date);
  if (Number.isNaN(at.getTime())) throw new TypeError(`not a date: ${date}`);
  return at.getHours() * 60 + at.getMinutes();
}

export function isOpen(window, date) {
  const minutes = minuteOfDay(date);
  if (window.overnight) return minutes >= window.from || minutes < window.to;
  return minutes >= window.from && minutes < window.to;
}

export function minutesUntilOpen(window, date) {
  const minutes = minuteOfDay(date);
  if (isOpen(window, date)) return 0;
  return (window.from - minutes + MINUTES_PER_DAY) % MINUTES_PER_DAY;
}

export function nextOpen(window, date) {
  const at = date instanceof Date ? date : new Date(date);
  const wait = minutesUntilOpen(window, at);
  return new Date(at.getTime() + wait * 60_000).toISOString();
}

export function parseQuietHours(entries) {
  if (entries === undefined || entries === null || entries === '') return [];
  const list = Array.isArray(entries) ? entries : String(entries).split(',');
  return list
    .map((entry) => String(entry).trim())
    .filter(Boolean)
    .map((entry) => parseWindow(entry));
}

export function inQuietHours(windows, date) {
  return (windows ?? []).some((window) => isOpen(window, date));
}

export function describeWindow(window) {
  return `${formatClock(window.from)}-${formatClock(window.to)}${window.overnight ? ' (overnight)' : ''}`;
}

export function businessWindow() {
  return parseWindow('09:00-17:00');
}

export function weekdayOf(date) {
  const at = date instanceof Date ? date : new Date(date);
  return WEEKDAYS[(at.getDay() + 6) % 7];
}

export function isWeekend(date) {
  const day = weekdayOf(date);
  return day === 'sat' || day === 'sun';
}
