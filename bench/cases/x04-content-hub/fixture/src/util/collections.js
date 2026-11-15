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
  for (let index = 0; index < items.length; index += width) {
    out.push(items.slice(index, index + width));
  }
  return out;
}

export function sortBy(items, keyOf, direction = 'asc') {
  const sign = direction === 'desc' ? -1 : 1;
  return [...(items ?? [])].sort((left, right) => {
    const a = keyOf(left);
    const b = keyOf(right);
    if (a === b) return 0;
    if (a === undefined || a === null) return -1 * sign;
    if (b === undefined || b === null) return sign;
    return a < b ? -sign : sign;
  });
}

export function paginate(items, { limit = 20, offset = 0 } = {}) {
  const size = Math.max(1, Math.min(200, Math.trunc(limit)));
  const start = Math.max(0, Math.trunc(offset));
  const slice = items.slice(start, start + size);
  const total = items.length;
  return {
    items: slice,
    total,
    limit: size,
    offset: start,
    has_more: start + slice.length < total,
    next_offset: start + slice.length < total ? start + slice.length : null,
  };
}

export function flattenTree(nodes, childrenOf = (node) => node.children ?? []) {
  const out = [];
  const walk = (node, depth) => {
    out.push({ node, depth });
    for (const child of childrenOf(node)) walk(child, depth + 1);
  };
  for (const node of nodes ?? []) walk(node, 0);
  return out;
}

export function partition(items, predicate) {
  const yes = [];
  const no = [];
  for (const item of items ?? []) (predicate(item) ? yes : no).push(item);
  return [yes, no];
}

export function mapValues(value, transform) {
  const out = {};
  for (const [key, item] of Object.entries(value ?? {})) out[key] = transform(item, key);
  return out;
}

export function sumBy(items, keyOf) {
  let total = 0;
  for (const item of items ?? []) total += Number(keyOf(item)) || 0;
  return total;
}

export function minBy(items, keyOf) {
  let best;
  for (const item of items ?? []) {
    if (best === undefined || keyOf(item) < keyOf(best)) best = item;
  }
  return best;
}

export function maxBy(items, keyOf) {
  let best;
  for (const item of items ?? []) {
    if (best === undefined || keyOf(item) > keyOf(best)) best = item;
  }
  return best;
}

export function indexWhere(items, predicate) {
  for (let index = 0; index < (items?.length ?? 0); index += 1) {
    if (predicate(items[index])) return index;
  }
  return -1;
}

export function topN(items, keyOf, count = 5) {
  return sortBy(items, keyOf, 'desc').slice(0, Math.max(0, count));
}

export function tally(values) {
  const out = new Map();
  for (const value of values ?? []) out.set(value, (out.get(value) ?? 0) + 1);
  return [...out.entries()].sort((left, right) => right[1] - left[1]);
}

export function sliding(items, size) {
  const width = Math.max(1, Math.trunc(size));
  const out = [];
  for (let index = 0; index + width <= items.length; index += 1) {
    out.push(items.slice(index, index + width));
  }
  return out;
}

export function diffLists(before, after, keyOf = (item) => item) {
  const beforeKeys = new Map((before ?? []).map((item) => [keyOf(item), item]));
  const afterKeys = new Map((after ?? []).map((item) => [keyOf(item), item]));
  const added = [...afterKeys.keys()].filter((key) => !beforeKeys.has(key));
  const removed = [...beforeKeys.keys()].filter((key) => !afterKeys.has(key));
  const kept = [...afterKeys.keys()].filter((key) => beforeKeys.has(key));
  return { added, removed, kept };
}

export function mergeById(left, right, keyOf = (item) => item.id) {
  const out = new Map();
  for (const item of left ?? []) out.set(keyOf(item), item);
  for (const item of right ?? []) out.set(keyOf(item), item);
  return [...out.values()];
}

export function zip(names, values) {
  const out = {};
  names.forEach((name, index) => {
    out[name] = values[index];
  });
  return out;
}

export function cartesian(lists) {
  return lists.reduce(
    (acc, list) => acc.flatMap((prefix) => list.map((value) => [...prefix, value])),
    [[]],
  );
}

export function rotate(items, by) {
  if (items.length === 0) return [];
  const offset = ((Math.trunc(by) % items.length) + items.length) % items.length;
  return [...items.slice(offset), ...items.slice(0, offset)];
}

export function takeWhile(items, predicate) {
  const out = [];
  for (const item of items ?? []) {
    if (!predicate(item)) break;
    out.push(item);
  }
  return out;
}

export function dropWhile(items, predicate) {
  let index = 0;
  while (index < (items?.length ?? 0) && predicate(items[index])) index += 1;
  return (items ?? []).slice(index);
}
