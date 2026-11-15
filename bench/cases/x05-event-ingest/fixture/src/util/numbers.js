export function toCents(value) {
  const number = Number(value);
  if (!Number.isFinite(number)) return undefined;
  return Math.round(number * 100);
}

export function fromCents(cents) {
  const value = Math.trunc(Number(cents));
  if (!Number.isFinite(value)) return undefined;
  const sign = value < 0 ? '-' : '';
  const abs = Math.abs(value);
  return `${sign}${Math.floor(abs / 100)}.${String(abs % 100).padStart(2, '0')}`;
}

export function clamp(value, min, max) {
  return Math.min(Math.max(value, min), max);
}

export function percentOf(part, whole) {
  if (!whole) return 0;
  return Math.round((part / whole) * 10000) / 100;
}

export function ratePerThousand(units, rateCents) {
  return Math.round((units * rateCents) / 1000);
}

export function parseInteger(text, fallback) {
  const value = Number.parseInt(String(text ?? ''), 10);
  return Number.isFinite(value) ? value : fallback;
}

export function parseFloatSafe(text, fallback) {
  const value = Number.parseFloat(String(text ?? ''));
  return Number.isFinite(value) ? value : fallback;
}

export function sum(values) {
  let total = 0;
  for (const value of values ?? []) total += Number(value) || 0;
  return total;
}

export function mean(values) {
  const list = [...(values ?? [])].filter((value) => Number.isFinite(value));
  return list.length === 0 ? 0 : sum(list) / list.length;
}

export function median(values) {
  const list = [...(values ?? [])].filter((value) => Number.isFinite(value)).sort((a, b) => a - b);
  if (list.length === 0) return 0;
  const middle = Math.floor(list.length / 2);
  return list.length % 2 === 0 ? (list[middle - 1] + list[middle]) / 2 : list[middle];
}

export function quantile(values, fraction) {
  const list = [...(values ?? [])].filter((value) => Number.isFinite(value)).sort((a, b) => a - b);
  if (list.length === 0) return 0;
  const index = clamp(Math.floor(fraction * list.length), 0, list.length - 1);
  return list[index];
}

export function formatBytes(bytes) {
  const size = Number(bytes) || 0;
  if (size < 1024) return `${size} B`;
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KiB`;
  if (size < 1024 * 1024 * 1024) return `${(size / (1024 * 1024)).toFixed(1)} MiB`;
  return `${(size / (1024 * 1024 * 1024)).toFixed(2)} GiB`;
}

export function formatNumber(value) {
  const number = Number(value) || 0;
  if (Math.abs(number) < 1000) return String(number);
  return number.toString().replace(/\B(?=(\d{3})+(?!\d))/g, ',');
}

export function safeDivide(numerator, denominator, fallback = 0) {
  if (!denominator) return fallback;
  const value = numerator / denominator;
  return Number.isFinite(value) ? value : fallback;
}

export function roundTo(value, places = 2) {
  const factor = 10 ** Math.max(0, Math.trunc(places));
  return Math.round((Number(value) || 0) * factor) / factor;
}

export function sumCentsOf(values) {
  return (values ?? []).reduce((total, value) => total + Math.trunc(Number(value) || 0), 0);
}

export function percentChange(before, after) {
  const left = Number(before) || 0;
  const right = Number(after) || 0;
  if (left === 0) return right === 0 ? 0 : 100;
  return roundTo(((right - left) / left) * 100, 2);
}

export function weightedMean(values, weights) {
  const list = (values ?? []).map(Number);
  const factors = (weights ?? []).map(Number);
  let total = 0;
  let weight = 0;
  for (const [index, value] of list.entries()) {
    const factor = Number.isFinite(factors[index]) ? factors[index] : 0;
    total += value * factor;
    weight += factor;
  }
  return weight === 0 ? 0 : roundTo(total / weight, 4);
}

export function bucketOfValue(value, edges) {
  const sorted = [...(edges ?? [])].sort((left, right) => left - right);
  for (const edge of sorted) if (Number(value) <= edge) return edge;
  return sorted[sorted.length - 1] ?? 0;
}

export function budgetShare(used, limit) {
  const cap = Number(limit) || 0;
  if (cap <= 0) return 0;
  return clamp(roundTo((Number(used) / cap) * 100, 2), 0, 100);
}
