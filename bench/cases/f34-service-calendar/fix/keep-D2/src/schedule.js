import { instant, monthParts, daysInMonth, format } from './dates.js';

export class Schedule {
  #entries = new Map();

  add(id, start, anchorDay) {
    if (this.#entries.has(id)) throw new Error('schedule exists');
    start = instant(start);
    const date = new Date(start);
    anchorDay ??= date.getUTCDate();
    if (!Number.isInteger(anchorDay) || anchorDay < 1 || anchorDay > 31) {
      throw new TypeError('anchor day');
    }
    this.#entries.set(id, { id, start, anchorDay, current: start, count: 0, active: true });
    return this.get(id);
  }

  get(id) {
    const entry = this.#entries.get(id);
    if (!entry) throw new Error('unknown schedule');
    return { ...entry };
  }

  next(id) {
    const entry = this.#entries.get(id);
    if (!entry || !entry.active) throw new Error('inactive schedule');
    const date = new Date(entry.current);
    const { year, month } = monthParts(entry.current);
    const nextMonth = month === 12 ? 1 : month + 1;
    const nextYear = month === 12 ? year + 1 : year;
    const day = Math.min(entry.anchorDay, daysInMonth(nextYear, nextMonth));
    entry.current = Date.UTC(nextYear, nextMonth - 1, day,
      date.getUTCHours(), date.getUTCMinutes(), date.getUTCSeconds(), date.getUTCMilliseconds());
    entry.count += 1;
    return format(entry.current);
  }

  pause(id) {
    this.get(id);
    this.#entries.get(id).active = false;
  }

  resume(id) {
    this.get(id);
    this.#entries.get(id).active = true;
  }

  list() {
    return [...this.#entries.keys()].sort().map(id => this.get(id));
  }

  preview(id, count) {
    if (!Number.isInteger(count) || count < 0 || count > 24) throw new TypeError('count');
    const entry = this.get(id);
    const clone = new Schedule();
    clone.#entries.set(id, { ...entry });
    return Array.from({ length: count }, () => clone.next(id));
  }
}
