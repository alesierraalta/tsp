export function clamp(value, min, max) {
  return Math.min(Math.max(value, min), max);
}

export function percentOf(part, whole) {
  if (!whole) return 0;
  return Math.round((part / whole) * 10000) / 100;
}

export function roundTo(value, places = 2) {
  const factor = 10 ** Math.max(0, Math.trunc(places));
  return Math.round((Number(value) || 0) * factor) / factor;
}

export function safeDivide(numerator, denominator, fallback = 0) {
  if (!denominator) return fallback;
  const value = numerator / denominator;
  return Number.isFinite(value) ? value : fallback;
}

export function sum(values) {
  let total = 0;
  for (const value of values ?? []) total += Number(value) || 0;
  return total;
}

export function mean(values) {
  const list = [...(values ?? [])].filter((value) => Number.isFinite(Number(value))).map(Number);
  return list.length === 0 ? 0 : roundTo(sum(list) / list.length, 4);
}

export function median(values) {
  const list = [...(values ?? [])].map(Number).filter(Number.isFinite).sort((left, right) => left - right);
  if (list.length === 0) return 0;
  const middle = Math.floor(list.length / 2);
  return list.length % 2 === 0 ? roundTo((list[middle - 1] + list[middle]) / 2, 4) : list[middle];
}

export function quantile(values, fraction) {
  const list = [...(values ?? [])].map(Number).filter(Number.isFinite).sort((left, right) => left - right);
  if (list.length === 0) return 0;
  const index = clamp(Math.floor(fraction * list.length), 0, list.length - 1);
  return list[index];
}

export function formatNumber(value) {
  const number = Number(value) || 0;
  if (Math.abs(number) < 1000) return String(number);
  return number.toString().replace(/\B(?=(\d{3})+(?!\d))/g, ',');
}

export function formatBytes(bytes) {
  const size = Number(bytes) || 0;
  if (size < 1024) return `${size} B`;
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KiB`;
  if (size < 1024 * 1024 * 1024) return `${(size / (1024 * 1024)).toFixed(1)} MiB`;
  return `${(size / (1024 * 1024 * 1024)).toFixed(2)} GiB`;
}

export function bucketOfValue(value, edges) {
  const sorted = [...(edges ?? [])].sort((left, right) => left - right);
  for (const edge of sorted) if (Number(value) <= edge) return edge;
  return sorted[sorted.length - 1] ?? 0;
}

export function percentChange(before, after) {
  const left = Number(before) || 0;
  const right = Number(after) || 0;
  if (left === 0) return right === 0 ? 0 : 100;
  return roundTo(((right - left) / left) * 100, 2);
}

export function minutesBetween(a, b) {
  const left = Number(a) || 0;
  const right = Number(b) || 0;
  return roundTo((right - left) / 60000, 2);
}

export function toBasisPoints(ratio) {
  return Math.round((Number(ratio) || 0) * 10000);
}

export function describePercent(value) {
  return `${roundTo(value, 1)}%`;
}

export function percentiles(values, fractions = [0.5, 0.9, 0.99]) {
  return (fractions ?? []).map((fraction) => ({ fraction, value: quantile(values, fraction) }));
}

export function rateOf(part, whole) {
  return safeDivide(Number(part) || 0, Number(whole) || 0, 0);
}

export function describeRate(part, whole) {
  return `${roundTo(rateOf(part, whole) * 100, 1)}%`;
}

export function bounded(value, min, max, fallback = min) {
  const number = Number(value);
  if (!Number.isFinite(number)) return fallback;
  return clamp(number, min, max);
}
