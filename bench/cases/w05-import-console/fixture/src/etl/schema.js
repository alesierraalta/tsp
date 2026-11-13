import { schemaError } from '../util/errors.js';

export const COLUMN_TYPES = ['text', 'integer', 'money', 'date', 'enum', 'email'];

const EMAIL_PATTERN = /^[^\s@]+@[^\s@]+\.[a-z]{2,}$/i;

function column(name, type, extra = {}) {
  if (typeof name !== 'string' || name.length === 0) throw new TypeError('a column needs a name');
  if (!COLUMN_TYPES.includes(type)) throw new TypeError(`unknown column type: ${type}`);
  return {
    name,
    type,
    required: Boolean(extra.required),
    maxLength: extra.maxLength ?? 200,
    values: extra.values,
    default: extra.default,
  };
}

export const DATASETS = {
  items: {
    name: 'items',
    key: ['sku'],
    columns: [
      column('sku', 'text', { required: true, maxLength: 40 }),
      column('title', 'text', { required: true, maxLength: 200 }),
      column('category', 'text', { required: false, maxLength: 60 }),
      column('amount', 'money', { required: true }),
      column('stock', 'integer', { required: false }),
      column('updated_at', 'date', { required: false }),
    ],
  },
  orders: {
    name: 'orders',
    key: ['order_id'],
    columns: [
      column('order_id', 'text', { required: true, maxLength: 40 }),
      column('customer', 'email', { required: true, maxLength: 160 }),
      column('total', 'money', { required: true }),
      column('status', 'enum', { required: true, values: ['pending', 'paid', 'shipped', 'refunded'] }),
      column('placed_at', 'date', { required: true }),
    ],
  },
  customers: {
    name: 'customers',
    key: ['email'],
    columns: [
      column('email', 'email', { required: true, maxLength: 160 }),
      column('name', 'text', { required: true, maxLength: 120 }),
      column('country', 'text', { required: false, maxLength: 2 }),
      column('balance', 'money', { required: false }),
    ],
  },
};

export function schemaOf(name) {
  const schema = DATASETS[name];
  if (!schema) throw schemaError(`unknown dataset: ${name}`);
  return schema;
}

export function columnOf(schema, name) {
  const found = schema.columns.find((entry) => entry.name === name);
  if (!found) throw schemaError(`unknown column ${name} on ${schema.name}`);
  return found;
}

export function requiredColumns(schema) {
  return schema.columns.filter((entry) => entry.required).map((entry) => entry.name);
}

export function optionalColumns(schema) {
  return schema.columns.filter((entry) => !entry.required).map((entry) => entry.name);
}

export function keyColumns(schema) {
  return [...schema.key];
}

export function isKeyColumn(schema, name) {
  return schema.key.includes(name);
}

export function isEmail(value) {
  return EMAIL_PATTERN.test(String(value ?? ''));
}

export function coerce(type, value) {
  if (value === null || value === undefined) return null;
  const text = String(value).trim();
  if (text === '') return null;
  switch (type) {
    case 'text':
      return text;
    case 'integer':
      return Number.isFinite(Number(text)) ? Math.trunc(Number(text)) : null;
    case 'money':
      return text;
    case 'date':
      return text;
    case 'enum':
      return text.toLowerCase();
    case 'email':
      return text.toLowerCase();
    default:
      return text;
  }
}

export function columnSummary(schema) {
  return schema.columns.map((entry) => `${entry.name}:${entry.type}${entry.required ? '!' : '?'}`);
}

export function compareSchemas(left, right) {
  const leftNames = left.columns.map((entry) => entry.name);
  const rightNames = right.columns.map((entry) => entry.name);
  const missing = leftNames.filter((name) => !rightNames.includes(name));
  const extra = rightNames.filter((name) => !leftNames.includes(name));
  return { missing, extra, compatible: missing.length === 0 };
}
