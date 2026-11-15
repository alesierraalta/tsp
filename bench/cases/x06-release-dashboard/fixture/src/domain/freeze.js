import { dayBucket, parseDuration, toInstant } from '../util/time.js';
import { badRequest } from '../util/errors.js';

// A change freeze is a window in which production deployments are refused unless the release carries an
// explicit override. The window is evaluated against the deployment's start instant, not against the wall
// clock of the request, so a rollout queued before the freeze opened is still allowed to finish.
export const FREEZE_REASONS = ['holiday', 'incident', 'audit', 'migration', 'other'];

export function normalizeFreeze(input = {}, options = {}) {
  const from = toInstant(input.from);
  const to = toInstant(input.to);
  if (from === undefined || to === undefined) throw badRequest('invalid_freeze', 'a freeze needs a from and a to instant');
  if (from.getTime() >= to.getTime()) throw badRequest('invalid_freeze', 'a freeze ends after it starts');
  const environment = String(input.environment ?? 'production');
  const reason = FREEZE_REASONS.includes(input.reason) ? input.reason : 'other';
  return {
    id: String(input.id ?? `frz_${dayBucket(from)}_${environment}`),
    environment,
    from: from.toISOString(),
    to: to.toISOString(),
    reason,
    note: String(input.note ?? ''),
    created_by: String(input.created_by ?? options.actor ?? 'system'),
    override: input.override === true,
  };
}

export function covers(freeze, instant) {
  const at = toInstant(instant);
  if (at === undefined) return false;
  return at.getTime() >= new Date(freeze.from).getTime() && at.getTime() <= new Date(freeze.to).getTime();
}

export function durationMinutes(freeze) {
  return Math.round((new Date(freeze.to).getTime() - new Date(freeze.from).getTime()) / 60000);
}

export function describeFreeze(freeze) {
  return {
    id: freeze.id,
    environment: freeze.environment,
    reason: freeze.reason,
    from: freeze.from,
    to: freeze.to,
    minutes: durationMinutes(freeze),
    override: freeze.override,
  };
}

export class FreezeCalendar {
  constructor(options = {}) {
    this.clock = options.clock ?? (() => new Date());
    this.freezes = new Map();
  }

  add(input) {
    const freeze = normalizeFreeze(input, { actor: 'calendar' });
    this.freezes.set(freeze.id, freeze);
    return freeze;
  }

  remove(id) {
    return this.freezes.delete(String(id));
  }

  list(environment) {
    return [...this.freezes.values()]
      .filter((freeze) => environment === undefined || freeze.environment === environment)
      .sort((left, right) => (left.from < right.from ? -1 : 1));
  }

  active(environment, instant = this.clock()) {
    return this.list(environment).find((freeze) => covers(freeze, instant) && freeze.override !== true);
  }

  blocks(environment, instant) {
    return this.active(environment, instant) !== undefined;
  }

  next(environment, instant = this.clock()) {
    const at = toInstant(instant)?.getTime() ?? 0;
    return this.list(environment).find((freeze) => new Date(freeze.from).getTime() > at);
  }

  upcoming(environment, window = '7d') {
    const horizon = (toInstant(this.clock())?.getTime() ?? 0) + parseDuration(window, '7d');
    return this.list(environment).filter((freeze) => new Date(freeze.from).getTime() <= horizon);
  }

  stats() {
    const byReason = {};
    for (const freeze of this.freezes.values()) byReason[freeze.reason] = (byReason[freeze.reason] ?? 0) + 1;
    return { freezes: this.freezes.size, byReason, active: this.active('production') !== undefined };
  }
}

export function freezeReport(calendar, environment) {
  const list = calendar.list(environment);
  return {
    environment,
    freezes: list.length,
    minutes: list.reduce((total, freeze) => total + durationMinutes(freeze), 0),
    next: calendar.next(environment) === undefined ? null : describeFreeze(calendar.next(environment)),
    active: calendar.active(environment) === undefined ? null : describeFreeze(calendar.active(environment)),
  };
}
