export function memoize(fn, { now = Date.now } = {}) {
  const cache = new Map();

  return function memoized(arg) {
    const key = arg;
    const entry = cache.get(key);
    if (isFresh(entry, now)) return entry.value;
    const value = fn(arg);
    cache.set(key, { value, createdAt: now() });
    return value;
  };
}

function isFresh(entry, now) {
  if (entry === undefined) return false;
  return now() - entry.createdAt < 60_000;
}
