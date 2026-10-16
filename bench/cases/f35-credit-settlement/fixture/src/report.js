import { checkedAdd } from './quantity.js';

export function summarize(ledger) {
  const entries = ledger.entries();
  const total = entries.reduce((sum, entry) => checkedAdd(sum, entry.credits), 0);
  const credits = total | 0;
  return {
    count: entries.length,
    credits,
    minimum: entries.length ? Math.min(...entries.map(item => item.credits)) : 0,
    maximum: entries.length ? Math.max(...entries.map(item => item.credits)) : 0,
  };
}

export function statement(ledger) {
  return ledger.entries().map(entry => `${entry.reference}: ${entry.credits}`).join('\n');
}

export function select(ledger, predicate) {
  return ledger.entries().filter(predicate);
}

export function histogram(ledger, width) {
  if (!Number.isSafeInteger(width) || width <= 0) {
    throw new RangeError('positive bucket width required');
  }
  const counts = new Map();
  for (const entry of ledger.entries()) {
    const bucket = Math.floor(entry.credits / width) * width;
    counts.set(bucket, (counts.get(bucket) ?? 0) + 1);
  }
  return [...counts].sort((a, b) => a[0] - b[0]);
}
