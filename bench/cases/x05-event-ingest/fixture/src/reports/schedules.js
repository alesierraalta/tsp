import { badRequest } from '../util/errors.js';
import { addDays, dayBucket, isWithin } from '../util/time.js';

export const REPORT_KINDS = ['rollup', 'sessions', 'billing', 'retention'];

export function normalizeSchedule(input = {}) {
  const kind = REPORT_KINDS.includes(input.kind) ? input.kind : 'rollup';
  const hour = Number(input.hour ?? 6);
  if (!Number.isInteger(hour) || hour < 0 || hour > 23) throw badRequest('invalid_schedule', 'hour must be 0 to 23');
  return {
    id: String(input.id ?? `sch_${kind}`),
    tenant: String(input.tenant ?? 'default'),
    kind,
    hour,
    cadence: input.cadence === 'weekly' ? 'weekly' : 'daily',
    recipients: Array.isArray(input.recipients) ? input.recipients.map(String) : [],
    enabled: input.enabled !== false,
  };
}

export class ScheduleBook {
  constructor(options = {}) {
    this.clock = options.clock ?? (() => new Date());
    this.schedules = new Map();
    this.sent = [];
  }

  add(input) {
    const schedule = normalizeSchedule(input);
    this.schedules.set(schedule.id, schedule);
    return schedule;
  }

  due(at) {
    const instant = at ?? new Date(this.clock());
    return [...this.schedules.values()].filter((schedule) => {
      if (!schedule.enabled) return false;
      if (schedule.cadence === 'weekly' && instant.getUTCDay() !== 1) return false;
      return instant.getUTCHours() >= schedule.hour;
    });
  }

  markSent(schedule, at) {
    const record = { schedule: schedule.id, at: at ?? new Date(this.clock()).toISOString(), day: dayBucket(at ?? this.clock()) };
    this.sent.push(record);
    return record;
  }

  sentToday(tenant, at) {
    const day = dayBucket(at ?? this.clock());
    return this.sent.filter((record) => record.day === day && this.schedules.get(record.schedule)?.tenant === tenant);
  }

  list(tenant) {
    return [...this.schedules.values()].filter((schedule) => tenant === undefined || schedule.tenant === tenant);
  }

  within(from, to) {
    return this.sent.filter((record) => isWithin(record.at, from, to));
  }

  next(schedule) {
    const base = new Date(this.clock());
    const next = new Date(Date.UTC(base.getUTCFullYear(), base.getUTCMonth(), base.getUTCDate(), schedule.hour));
    if (next.getTime() <= base.getTime()) next.setUTCDate(next.getUTCDate() + 1);
    if (schedule.cadence === 'weekly' && next.getUTCDay() !== 1) {
      return addDays(next, (8 - next.getUTCDay()) % 7 || 7).toISOString();
    }
    return next.toISOString();
  }

  stats() {
    return { schedules: this.schedules.size, sent: this.sent.length };
  }
}
