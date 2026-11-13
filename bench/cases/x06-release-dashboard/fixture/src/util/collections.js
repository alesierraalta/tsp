export function groupBy(items, keyOf) {
  const out = new Map();
  for (const item of items ?? []) {
    const key = keyOf(item);
    const bucket = out.get(key);
    if (bucket) bucket.push(item);
    else out.set(key, [item]);
  }
  return out;
}

export function countBy(items, keyOf) {
  const out = new Map();
  for (const item of items ?? []) {
    const key = keyOf(item);
    out.set(key, (out.get(key) ?? 0) + 1);
  }
  return out;
}

export function keyBy(items, keyOf) {
  const out = new Map();
  for (const item of items ?? []) out.set(keyOf(item), item);
  return out;
}

export function uniqueBy(items, keyOf) {
  const seen = new Set();
  const out = [];
  for (const item of items ?? []) {
    const key = keyOf(item);
    if (seen.has(key)) continue;
    seen.add(key);
    out.push(item);
  }
  return out;
}

export function chunk(items, size) {
  const width = Math.max(1, Math.trunc(size));
  const out = [];
  for (let index = 0; index < (items?.length ?? 0); index += width) out.push(items.slice(index, index + width));
  return out;
}

export function sortBy(items, keyOf, direction = 'asc') {
  const sign = direction === 'desc' ? -1 : 1;
  return [...(items ?? [])].sort((left, right) => {
    const a = keyOf(left);
    const b = keyOf(right);
    if (a === b) return 0;
    if (a === undefined || a === null) return -sign;
    if (b === undefined || b === null) return sign;
    return a < b ? -sign : sign;
  });
}

export function paginate(items, options = {}) {
  const limit = Math.max(1, Math.min(200, Math.trunc(options.limit ?? 50)));
  const offset = Math.max(0, Math.trunc(options.offset ?? 0));
  const slice = (items ?? []).slice(offset, offset + limit);
  return {
    items: slice,
    total: (items ?? []).length,
    limit,
    offset,
    has_more: offset + slice.length < (items ?? []).length,
  };
}

export function sumBy(items, keyOf) {
  let total = 0;
  for (const item of items ?? []) total += Number(keyOf(item)) || 0;
  return total;
}

export function topBy(items, keyOf, count = 5) {
  return sortBy(items, keyOf, 'desc').slice(0, Math.max(0, count));
}

export function partition(items, predicate) {
  const yes = [];
  const no = [];
  for (const item of items ?? []) (predicate(item) ? yes : no).push(item);
  return [yes, no];
}

export function diffIds(before, after) {
  const left = new Set(before ?? []);
  const right = new Set(after ?? []);
  return {
    added: [...right].filter((id) => !left.has(id)),
    removed: [...left].filter((id) => !right.has(id)),
    kept: [...right].filter((id) => left.has(id)),
  };
}

export function tally(values) {
  const out = new Map();
  for (const value of values ?? []) out.set(value, (out.get(value) ?? 0) + 1);
  return [...out.entries()].sort((left, right) => right[1] - left[1]);
}

export function windowed(items, size, step = 1) {
  const width = Math.max(1, Math.trunc(size));
  const stride = Math.max(1, Math.trunc(step));
  const out = [];
  for (let index = 0; index + width <= (items?.length ?? 0); index += stride) out.push(items.slice(index, index + width));
  return out;
}

export function zipRows(left, right, keyOf) {
  const lookup = new Map((right ?? []).map((item) => [keyOf(item), item]));
  return (left ?? []).map((item) => ({ item, match: lookup.get(keyOf(item)) ?? null }));
}

export function spread(items, keyOf) {
  const values = (items ?? []).map((item) => Number(keyOf(item)) || 0);
  if (values.length === 0) return { min: 0, max: 0, range: 0 };
  const min = Math.min(...values);
  const max = Math.max(...values);
  return { min, max, range: max - min };
}

export function mode(values) {
  const counts = countBy(values ?? [], (value) => value);
  const best = [...counts.entries()].sort((left, right) => right[1] - left[1])[0];
  return best === undefined ? undefined : { value: best[0], count: best[1] };
}

export function flattenGroups(groups) {
  const out = [];
  for (const [label, rows] of (groups ?? new Map()).entries()) {
    for (const row of rows) out.push({ group: label, row });
  }
  return out;
}

export function mapValues(value, transform) {
  return Object.fromEntries(Object.entries(value ?? {}).map(([name, item]) => [name, transform(item, name)]));
}

export function invert(index) {
  const out = new Map();
  for (const [name, item] of (index ?? new Map()).entries()) out.set(item, name);
  return out;
}
