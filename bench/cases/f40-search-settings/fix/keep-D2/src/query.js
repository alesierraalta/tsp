export function normalizeQuery(value) {
  if (typeof value !== 'string') {
    throw new TypeError('query must be text');
  }
  return value.replace(/[\u0000-\u001f\u007f]/g, ' ')
    .replace(/\s+/gu, ' ').trim();
}

export function interpretQuery(value, settings) {
  const normalized = normalizeQuery(value);
  const text = normalized.slice(0, settings.maxQueryLength).trimEnd();
  const terms = text.toLocaleLowerCase('en').split(' ').filter(Boolean);
  return Object.freeze({
    text,
    terms: Object.freeze(terms),
    empty: terms.length === 0,
    mode: settings.matchMode,
  });
}

export function matchesQuery(document, query) {
  if (query.empty) {
    return true;
  }
  const haystack = `${document.title} ${document.body}`.toLocaleLowerCase('en');
  if (query.mode === 'phrase') {
    return haystack.includes(query.text.toLocaleLowerCase('en'));
  }
  if (query.mode === 'any') {
    return query.terms.some(term => haystack.includes(term));
  }
  return query.terms.every(term => haystack.includes(term));
}

export function querySummary(query) {
  return {
    text: query.text,
    termCount: query.terms.length,
    mode: query.mode,
    empty: query.empty,
  };
}
