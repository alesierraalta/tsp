export function normalizeWeights(weights) {
  if (!Array.isArray(weights) || weights.length === 0) {
    throw new TypeError('weights must be a non empty list');
  }
  const cleaned = weights.map((weight) => {
    const value = Number(weight);
    if (!Number.isFinite(value) || value < 0) throw new TypeError('weights must be zero or more');
    return value;
  });
  const total = cleaned.reduce((sum, value) => sum + value, 0);
  if (total <= 0) throw new TypeError('at least one weight must be positive');
  return cleaned.map((value) => value / total);
}

export function allocate(cents, weights) {
  if (!Number.isInteger(cents)) throw new TypeError('cents must be a whole number');
  const shares = normalizeWeights(weights);
  const sign = cents < 0 ? -1 : 1;
  const absolute = Math.abs(cents);
  const raw = shares.map((share) => absolute * share);
  const floors = raw.map((value) => Math.floor(value));
  const assigned = floors.reduce((sum, value) => sum + value, 0);
  const remainder = absolute - assigned;
  const order = raw
    .map((value, index) => ({ index, fraction: value - floors[index] }))
    .sort((left, right) => right.fraction - left.fraction || left.index - right.index);
  const result = [...floors];
  for (let index = 0; index < remainder; index += 1) {
    result[order[index % order.length].index] += 1;
  }
  return result.map((value) => sign * value);
}

export function allocateEvenly(cents, parts) {
  return allocate(cents, Array.from({ length: parts }, () => 1));
}

export function rebalance(rows, field, total) {
  const current = rows.reduce((sum, row) => sum + (row[field] ?? 0), 0);
  const drift = total - current;
  if (drift === 0 || rows.length === 0) return rows;
  const adjusted = [...rows];
  adjusted[adjusted.length - 1] = {
    ...adjusted[adjusted.length - 1],
    [field]: adjusted[adjusted.length - 1][field] + drift,
  };
  return adjusted;
}

export function assertBalanced(rows, field, total) {
  const current = rows.reduce((sum, row) => sum + (row[field] ?? 0), 0);
  if (current !== total) {
    throw new Error(`allocation is off by ${total - current} cents`);
  }
  return rows;
}

export function shareTable(cents, weights, labels) {
  const amounts = allocate(cents, weights);
  return amounts.map((amount, index) => ({
    label: labels?.[index] ?? `share-${index + 1}`,
    amount,
  }));
}
