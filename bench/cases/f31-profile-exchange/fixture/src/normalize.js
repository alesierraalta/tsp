import { checkFields, fields, text } from './schema.js';

export function compactRecord(input) {
  checkFields(input);
  const result = {};
  for (const field of Object.keys(input)) {
    const value = input[field];
    if (value !== null && value !== undefined) result[field] = value;
  }
  return result;
}

export function trimRecord(input) {
  checkFields(input);
  const result = {};
  for (const field of Object.keys(input)) {
    const value = text(input[field], field);
    result[field] = value === null ? null : value.trim();
  }
  return result;
}

export function fieldCounts(rows) {
  const counts = Object.fromEntries(fields.map(field => [field, 0]));
  for (const row of rows) {
    for (const field of fields) {
      if (row[field] !== null && row[field] !== undefined) counts[field]++;
    }
  }
  return counts;
}
