function integer(env, name, fallback, low, high) {
  const raw = env[name];
  if (raw === undefined || raw === '') {
    return fallback;
  }
  if (!/^\d+$/.test(raw)) {
    throw new Error(`${name} must be an integer`);
  }
  const value = Number(raw);
  if (!Number.isSafeInteger(value) || value < low || value > high) {
    throw new RangeError(`${name} is out of range`);
  }
  return value;
}

function choice(env, name, fallback, allowed) {
  const value = env[name] || fallback;
  if (!allowed.includes(value)) {
    throw new Error(`${name} is not supported`);
  }
  return value;
}

export function readSettings(env = process.env) {
  return Object.freeze({
    maxQueryLength: integer(env, 'SEARCH_MAX_QUERY_LENGTH', 80, 1, 4096),
    pageSize: integer(env, 'SEARCH_PAGE_SIZE', 20, 1, 100),
    matchMode: choice(env, 'SEARCH_MATCH_MODE', 'all', ['all', 'any', 'phrase']),
    sort: choice(env, 'SEARCH_SORT', 'relevance', ['relevance', 'title']),
  });
}

export function describeSettings(settings) {
  return {
    query: { maximum: settings.maxQueryLength, mode: settings.matchMode },
    results: { pageSize: settings.pageSize, sort: settings.sort },
  };
}

export function settingsEqual(left, right) {
  return left.maxQueryLength === right.maxQueryLength
    && left.pageSize === right.pageSize
    && left.matchMode === right.matchMode
    && left.sort === right.sort;
}
