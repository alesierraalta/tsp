export function text(value, name, max = 120) {
  if (typeof value !== 'string' || !value.trim() || value.length > max) {
    throw new TypeError(`invalid ${name}`);
  }
  return value.trim();
}

export function identifier(value) {
  const result = text(value, 'identifier', 64);
  if (!/^[a-zA-Z0-9_-]+$/.test(result)) {
    throw new TypeError('invalid identifier');
  }
  return result;
}

export function role(value) {
  if (!['reader', 'editor', 'owner'].includes(value)) {
    throw new TypeError('invalid role');
  }
  return value;
}

export function title(value) {
  return text(value, 'title', 200);
}

export function labels(values) {
  if (!Array.isArray(values) || values.length > 20) {
    throw new TypeError('invalid labels');
  }
  return [...new Set(values.map(value => text(value, 'label', 32)))].sort();
}

export function page(options = {}) {
  const offset = options.offset ?? 0;
  const limit = options.limit ?? 20;
  if (!Number.isInteger(offset) || offset < 0) throw new TypeError('offset');
  if (!Number.isInteger(limit) || limit < 1 || limit > 100) throw new TypeError('limit');
  return { offset, limit };
}

export function copy(value) {
  return structuredClone(value);
}

export function timestamp(value) {
  if (!Number.isSafeInteger(value) || value < 0) {
    throw new TypeError('invalid timestamp');
  }
  return value;
}
