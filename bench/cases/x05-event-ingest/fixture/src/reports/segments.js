import { dayBucket, toInstant } from '../util/time.js';
import { groupBy } from '../util/collections.js';

export function segmentRules(input = {}) {
  const rules = [];
  if (input.channel) rules.push({ field: 'channel', value: String(input.channel) });
  if (input.path_prefix) rules.push({ field: 'path_prefix', value: String(input.path_prefix) });
  if (input.user_property) rules.push({ field: String(input.user_property), value: input.value });
  if (input.since_days) rules.push({ field: 'since_days', value: Math.max(1, Math.trunc(Number(input.since_days))) });
  return rules;
}

export function matchesSegment(event, rules, clock = () => new Date()) {
  for (const rule of rules ?? []) {
    switch (rule.field) {
      case 'channel': {
        const referrer = String(event.context?.referrer ?? '').toLowerCase();
        const channel = referrer.includes('google') ? 'organic' : referrer === '' ? 'direct' : 'referral';
        if (channel !== rule.value) return false;
        break;
      }
      case 'path_prefix':
        if (!String(event.context?.path ?? '').startsWith(rule.value)) return false;
        break;
      case 'since_days': {
        const cutoff = new Date(clock()).getTime() - rule.value * 86400000;
        if ((toInstant(event.at)?.getTime() ?? 0) < cutoff) return false;
        break;
      }
      default:
        if (String(event.context?.[rule.field] ?? event.properties?.[rule.field] ?? '') !== String(rule.value)) return false;
    }
  }
  return true;
}

export class SegmentBook {
  constructor(options = {}) {
    this.clock = options.clock ?? (() => new Date());
    this.segments = new Map();
  }

  define(name, input) {
    const segment = { name: String(name), rules: segmentRules(input), created_at: new Date(this.clock()).toISOString() };
    this.segments.set(segment.name, segment);
    return segment;
  }

  get(name) {
    return this.segments.get(String(name));
  }

  members(name, events) {
    const segment = this.get(name);
    if (!segment) return [];
    return (events ?? []).filter((event) => matchesSegment(event, segment.rules, this.clock));
  }

  size(name, events) {
    return new Set(this.members(name, events).map((event) => event.context?.user ?? event.id)).size;
  }

  list() {
    return [...this.segments.values()];
  }

  remove(name) {
    return this.segments.delete(String(name));
  }

  overlap(name, otherName, events) {
    const left = new Set(this.members(name, events).map((event) => event.context?.user ?? event.id));
    const right = new Set(this.members(otherName, events).map((event) => event.context?.user ?? event.id));
    const shared = [...left].filter((user) => right.has(user));
    return { left: left.size, right: right.size, shared: shared.length };
  }

  stats(events) {
    return {
      segments: this.segments.size,
      sizes: Object.fromEntries(this.list().map((segment) => [segment.name, this.size(segment.name, events)])),
    };
  }
}

export function dailyReach(events, users) {
  const days = groupBy(events ?? [], (event) => dayBucket(event.at));
  return [...days.entries()]
    .map(([day, rows]) => ({ day, reached: rows.filter((row) => users.includes(row.context?.user ?? row.id)).length }))
    .sort((left, right) => (left.day < right.day ? -1 : 1));
}
