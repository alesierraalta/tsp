import { addMinutes, diffMinutes, toInstant } from '../util/time.js';
import { badRequest } from '../util/errors.js';

export const SCHEDULES = {
  digest: { every_minutes: 60, kind: 'digest' },
  collect: { every_minutes: 15, kind: 'collect' },
  sweep: { every_minutes: 1440, kind: 'sweep' },
};

export function normalizeSchedule(input = {}) {
  const kind = String(input.kind ?? 'digest');
  if (!Object.prototype.hasOwnProperty.call(SCHEDULES, kind)) throw badRequest('invalid_schedule', `${kind} cannot be scheduled`);
  const everyMinutes = Math.max(1, Math.trunc(Number(input.every_minutes ?? SCHEDULES[kind].every_minutes)));
  return { kind, every_minutes: everyMinutes, enabled: input.enabled !== false, last_run: input.last_run ?? null };
}

export class Scheduler {
  constructor(options = {}) {
    this.clock = options.clock ?? (() => new Date());
    this.queue = options.queue;
    this.schedules = new Map();
    for (const kind of Object.keys(SCHEDULES)) this.schedules.set(kind, normalizeSchedule({ kind }));
  }

  define(input) {
    const schedule = normalizeSchedule(input);
    this.schedules.set(schedule.kind, schedule);
    return schedule;
  }

  get(kind) {
    return this.schedules.get(String(kind));
  }

  nextRun(schedule) {
    const base = schedule.last_run ?? new Date(this.clock()).toISOString();
    return addMinutes(base, schedule.every_minutes).toISOString();
  }

  due() {
    const now = new Date(this.clock()).getTime();
    return [...this.schedules.values()].filter((schedule) => schedule.enabled && toInstant(this.nextRun(schedule)).getTime() <= now);
  }

  tick(options = {}) {
    if (!this.queue) throw new TypeError('a scheduler needs a queue');
    const due = this.due();
    const enqueued = due.map((schedule) => {
      const job = this.queue.enqueue(schedule.kind, options.payload ?? {}, { priority: 1 });
      schedule.last_run = new Date(this.clock()).toISOString();
      return job;
    });
    return { scheduled: due.length, jobs: enqueued.map((job) => job.id) };
  }

  list() {
    return [...this.schedules.values()].map((schedule) => ({
      kind: schedule.kind,
      every_minutes: schedule.every_minutes,
      enabled: schedule.enabled,
      next_run: this.nextRun(schedule),
    }));
  }

  stats() {
    return { schedules: this.schedules.size, enabled: this.list().filter((schedule) => schedule.enabled).length };
  }
}

export function minutesUntil(schedule, clock = () => new Date()) {
  const base = schedule.last_run ?? new Date(clock()).toISOString();
  const elapsed = diffMinutes(base, new Date(clock()));
  return Math.max(0, schedule.every_minutes - elapsed);
}
