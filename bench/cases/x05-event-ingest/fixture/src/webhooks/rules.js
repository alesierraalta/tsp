import { assertTarget, ALLOWED_HOSTS } from './targets.js';
import { badRequest } from '../util/errors.js';

export function normalizeRule(input = {}) {
  const event = String(input.event ?? 'batch.accepted');
  const url = String(input.url ?? '');
  if (url === '') throw badRequest('invalid_rule', 'a rule needs a url');
  return {
    id: String(input.id ?? `rule_${event}`),
    event,
    url,
    threshold: Number.isFinite(Number(input.threshold)) ? Number(input.threshold) : 0,
    enabled: input.enabled !== false,
  };
}

export class RuleBook {
  constructor(options = {}) {
    this.targets = options.targets ?? { allowed: ALLOWED_HOSTS };
    this.rules = new Map();
  }

  add(input) {
    const rule = normalizeRule(input);
    assertTarget(rule.url, { allowed: this.targets.allowed });
    this.rules.set(rule.id, rule);
    return rule;
  }

  list() {
    return [...this.rules.values()];
  }

  matching(event, value = 0) {
    return this.list().filter(
      (rule) => rule.enabled && rule.event === event && value >= rule.threshold,
    );
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

  remove(id) {
    return this.rules.delete(String(id));
  }

  stats() {
    return { rules: this.rules.size, enabled: this.list().filter((rule) => rule.enabled).length };
  }
}

export function rulesFor(rules, event) {
  return rules.filter((rule) => rule.event === event);
}

export function evaluateRules(rules, event, value = 0) {
  const matched = [];
  const skipped = [];
  for (const rule of rules ?? []) {
    if (rule.event !== event) continue;
    if (!rule.enabled) {
      skipped.push({ id: rule.id, reason: 'disabled' });
      continue;
    }
    if (value < rule.threshold) {
      skipped.push({ id: rule.id, reason: 'below_threshold', threshold: rule.threshold });
      continue;
    }
    matched.push({ id: rule.id, url: rule.url, threshold: rule.threshold });
  }
  return { matched, skipped };
}

export function ruleReport(rules) {
  const byEvent = {};
  for (const rule of rules ?? []) byEvent[rule.event] = (byEvent[rule.event] ?? 0) + 1;
  return {
    rules: (rules ?? []).length,
    enabled: (rules ?? []).filter((rule) => rule.enabled).length,
    byEvent,
  };
}

export function thresholdsOf(rules) {
  return Object.fromEntries((rules ?? []).map((rule) => [rule.id, rule.threshold]));
}

export function mergeRules(rules, extra) {
  const byId = new Map((rules ?? []).map((rule) => [rule.id, rule]));
  for (const rule of extra ?? []) byId.set(rule.id, rule);
  return [...byId.values()];
}

export function staleRules(rules, days, clock = () => new Date()) {
  const cutoff = new Date(clock()).getTime() - Math.max(0, Math.trunc(days)) * 86400000;
  return (rules ?? []).filter((rule) => rule.updated_at !== undefined && new Date(rule.updated_at).getTime() < cutoff);
}
