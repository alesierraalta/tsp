export function options(input = {}) {
  const value = {
    chunkSize: input.chunkSize ?? 64,
    attempts: input.attempts ?? 3,
    maxBytes: input.maxBytes ?? 1024 * 1024,
    namespace: input.namespace ?? 'default',
  };
  for (const key of ['chunkSize', 'attempts', 'maxBytes']) {
    if (!Number.isSafeInteger(value[key]) || value[key] <= 0) {
      throw new RangeError(`${key} must be a positive integer`);
    }
  }
  if (typeof value.namespace !== 'string' || !value.namespace.trim()) {
    throw new TypeError('namespace must be nonempty');
  }
  return Object.freeze(value);
}

export function identifier(value) {
  if (typeof value !== 'string' || !/^[a-zA-Z0-9_-]+$/.test(value)) {
    throw new TypeError('invalid identifier');
  }
  return value;
}

export function payload(value, limit) {
  if (typeof value !== 'string') {
    throw new TypeError('payload must be text');
  }
  const bytes = Buffer.from(value, 'utf8');
  if (bytes.length > limit) {
    throw new RangeError('payload exceeds limit');
  }
  return bytes;
}
