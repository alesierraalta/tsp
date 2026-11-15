import { newId } from '../util/ids.js';
import { nowIso } from '../util/time.js';

export const ALERT_KINDS = ['spike', 'silence', 'error_rate', 'quota'];

export function normalizeAlert(input = {}) {
  const kind = ALERT_KINDS.includes(input.kind) ? input.kind : 'spike';
  const threshold = Number(input.threshold);
  if (!Number.isFinite(threshold) || threshold <= 0) throw new TypeError('an alert needs a positive threshold');
  return {
    id: String(input.id ?? `alrt_${kind}`),
    kind,
    event: String(input.event ?? 'page_view'),
    threshold,
    window: String(input.window ?? 'hour'),
    enabled: input.enabled !== false,
  };
}

export class AlertBook {
  constructor(options = {}) {
    this.clock = options.clock ?? (() => new Date());
    this.rules = new Map();
    this.fired = [];
  }

  add(input) {
    const rule = normalizeAlert(input);
    this.rules.set(rule.id, rule);
    return rule;
  }

  evaluate(rule, value) {
    if (!rule.enabled) return undefined;
    if (rule.kind === 'silence' ? value <= rule.threshold : value >= rule.threshold) {
      const alert = {
        id: newId('fire'),
        rule: rule.id,
        kind: rule.kind,
        event: rule.event,
        value,
        threshold: rule.threshold,
        at: nowIso(this.clock),
      };
      this.fired.push(alert);
      return alert;
    }
    return undefined;
  }

  check(event, values = {}) {
    const out = [];
    for (const rule of this.rules.values()) {
      if (rule.event !== event) continue;
      const value = Number(values[rule.window] ?? 0);
      const alert = this.evaluate(rule, value);
      if (alert) out.push(alert);
    }
    return out;
  }

  list() {
    return [...this.rules.values()];
  }

  history(limit = 25) {
    return this.fired.slice(-limit).reverse();
  }

  enable(id) {
    const rule = this.rules.get(String(id));
    if (!rule) return undefined;
    rule.enabled = true;
    return rule;
  }

  disable(id) {
    const rule = this.rules.get(String(id));
    if (!rule) return undefined;
    rule.enabled = false;
    return rule;
  }

  stats() {
    const byKind = {};
    for (const alert of this.fired) byKind[alert.kind] = (byKind[alert.kind] ?? 0) + 1;
    return { rules: this.rules.size, fired: this.fired.length, byKind };
  }
}

export function thresholdOf(series, kind) {
  const values = (series ?? []).map((point) => point.events ?? 0);
  if (values.length === 0) return 0;
  const sorted = [...values].sort((left, right) => left - right);
  if (kind === 'spike') return sorted[Math.floor(sorted.length * 0.95)] ?? sorted[sorted.length - 1];
  return sorted[0];
}

export const SILENCE_SECONDS = { hour: 3600, day: 86400, week: 604800 };

export function silenceSeconds(window) {
  const label = String(window ?? 'hour');
  return SILENCE_SECONDS[label] ?? SILENCE_SECONDS.hour;
}

export function isMuted(rule, clock = () => new Date()) {
  if (!rule?.muted_until) return false;
  return new Date(rule.muted_until).getTime() > new Date(clock()).getTime();
}

export function muteRule(book, id, window = 'hour', clock = () => new Date()) {
  const rule = book.rules.get(String(id));
  if (!rule) return undefined;
  rule.muted_until = new Date(new Date(clock()).getTime() + silenceSeconds(window) * 1000).toISOString();
  return rule;
}

export function unmuteRule(book, id) {
  const rule = book.rules.get(String(id));
  if (!rule) return undefined;
  delete rule.muted_until;
  return rule;
}

// Evaluating a series walks the points a report already built, so an alert fires on the value the dashboard
// shows instead of on a second reading of the events. A muted rule is skipped without being removed.
export function evaluateSeries(book, event, points, clock = () => new Date()) {
  const fired = [];
  for (const rule of book.list()) {
    if (rule.event !== event || !rule.enabled || isMuted(rule, clock)) continue;
    for (const point of points ?? []) {
      const value = Number(point.events ?? 0);
      const alert = book.evaluate(rule, value);
      if (alert) fired.push({ ...alert, bucket: point.bucket });
    }
  }
  return fired;
}

export function alertDigest(book) {
  const stats = book.stats();
  const byEvent = {};
  for (const alert of book.fired) byEvent[alert.event] = (byEvent[alert.event] ?? 0) + 1;
  return { ...stats, byEvent, rules: book.list().length };
}

export function quietRules(book, clock = () => new Date()) {
  return book.list().filter((rule) => isMuted(rule, clock)).map((rule) => ({ id: rule.id, until: rule.muted_until }));
}

export function ruleWindows(rules, clock = () => new Date()) {
  const now = new Date(clock()).getTime();
  return (rules ?? []).map((rule) => {
    const seconds = silenceSeconds(rule.window);
    return { id: rule.id, window: rule.window, from: new Date(now - seconds * 1000).toISOString(), to: new Date(now).toISOString() };
  });
}
