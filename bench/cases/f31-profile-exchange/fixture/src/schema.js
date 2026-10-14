export const fields = Object.freeze(['id', 'name', 'email', 'team', 'note']);
export const mutable = Object.freeze(fields.filter(field => field !== 'id'));

export function identifier(value) {
  if (typeof value !== 'string' || !value.trim()) {
    throw new TypeError('id must be a nonempty string');
  }
  return value.trim();
}

export function text(value, field) {
  if (value === null) return null;
  if (typeof value !== 'string') {
    throw new TypeError(`${field} must be text or null`);
  }
  return value;
}

export function checkFields(input, allowed = fields) {
  if (!input || typeof input !== 'object' || Array.isArray(input)) {
    throw new TypeError('expected an object');
  }
  for (const field of Object.keys(input)) {
    if (!allowed.includes(field)) throw new TypeError(`unknown field: ${field}`);
  }
}

export function createRecord(input) {
  checkFields(input);
  const record = { id: identifier(input.id) };
  for (const field of mutable) {
    record[field] = input[field] === undefined ? null : text(input[field], field);
  }
  return record;
}

export function selection(columns) {
  if (!Array.isArray(columns) || columns.length === 0) {
    throw new TypeError('at least one column is required');
  }
  if (new Set(columns).size !== columns.length) throw new TypeError('duplicate column');
  for (const field of columns) {
    if (!fields.includes(field)) throw new TypeError(`unknown column: ${field}`);
  }
  return [...columns];
}
