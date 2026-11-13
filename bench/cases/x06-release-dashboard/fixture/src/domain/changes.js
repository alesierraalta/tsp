import { singleLine } from '../util/text.js';
import { badRequest } from '../util/errors.js';

export const CHANGE_KINDS = ['feature', 'fix', 'chore', 'security', 'breaking'];

export function normalizeChange(input = {}) {
  const summary = singleLine(input.summary ?? '');
  if (summary === '') throw badRequest('invalid_change', 'a change needs a summary');
  const kind = CHANGE_KINDS.includes(input.kind) ? input.kind : 'chore';
  return {
    kind,
    summary,
    reference: input.reference === undefined || input.reference === null ? null : String(input.reference),
    author: String(input.author ?? 'unknown'),
  };
}

export function normalizeChanges(list) {
  if (!Array.isArray(list)) return [];
  return list.map((entry) => normalizeChange(typeof entry === 'string' ? { summary: entry } : entry));
}

export function groupChanges(changes) {
  const out = {};
  for (const change of changes ?? []) {
    const bucket = out[change.kind] ?? [];
    bucket.push(change);
    out[change.kind] = bucket;
  }
  return out;
}

export function breakingChanges(changes) {
  return (changes ?? []).filter((change) => change.kind === 'breaking');
}

export function changeSummary(changes) {
  const byKind = {};
  for (const change of changes ?? []) byKind[change.kind] = (byKind[change.kind] ?? 0) + 1;
  return {
    changes: (changes ?? []).length,
    byKind,
    breaking: breakingChanges(changes).length,
    security: (changes ?? []).filter((change) => change.kind === 'security').length,
  };
}

export function changeLines(changes, marker = '-') {
  return (changes ?? []).map((change) => {
    const reference = change.reference === null ? '' : ` (${change.reference})`;
    return `${marker} ${change.kind}: ${change.summary}${reference}`;
  });
}

export function sortChanges(changes) {
  const order = CHANGE_KINDS;
  return [...(changes ?? [])].sort((left, right) => order.indexOf(left.kind) - order.indexOf(right.kind) || (left.summary < right.summary ? -1 : 1));
}

export function mergeChanges(...lists) {
  const seen = new Set();
  const out = [];
  for (const list of lists) {
    for (const change of list ?? []) {
      const key = `${change.kind}|${change.summary}`;
      if (seen.has(key)) continue;
      seen.add(key);
      out.push(change);
    }
  }
  return out;
}
