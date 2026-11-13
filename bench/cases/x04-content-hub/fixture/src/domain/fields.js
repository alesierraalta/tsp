import { normalizeWhitespace, singleLine, slugify } from '../util/text.js';

export const FIELD_TYPES = {
  text: { label: 'short text', max: 500 },
  long_text: { label: 'long text', max: 20000 },
  rich_text: { label: 'rich text', max: 50000 },
  number: { label: 'number' },
  integer: { label: 'whole number' },
  boolean: { label: 'boolean' },
  date: { label: 'date' },
  media: { label: 'media file' },
  reference: { label: 'reference' },
  json: { label: 'json document' },
  enum: { label: 'choice' },
  slug: { label: 'slug' },
};

export function isFieldType(type) {
  return Object.prototype.hasOwnProperty.call(FIELD_TYPES, type);
}

export function fieldLabel(field) {
  return field?.label ?? field?.name ?? 'field';
}

export function coerceValue(field, value) {
  switch (field.type) {
    case 'text':
    case 'slug':
      return singleLine(value).slice(0, field.max ?? FIELD_TYPES.text.max);
    case 'long_text':
    case 'rich_text':
      return normalizeWhitespace(value).slice(0, field.max ?? FIELD_TYPES[field.type].max);
    case 'number': {
      const number = Number(value);
      return Number.isFinite(number) ? number : undefined;
    }
    case 'integer': {
      const number = Number(value);
      return Number.isInteger(number) ? number : undefined;
    }
    case 'boolean':
      if (typeof value === 'boolean') return value;
      if (value === 'true') return true;
      if (value === 'false') return false;
      return undefined;
    case 'date': {
      const parsed = new Date(String(value ?? ''));
      return Number.isNaN(parsed.getTime()) ? undefined : parsed.toISOString();
    }
    case 'enum': {
      const text = singleLine(value);
      return (field.choices ?? []).includes(text) ? text : undefined;
    }
    case 'media':
    case 'reference':
      return typeof value === 'string' && value.trim() !== '' ? value.trim() : undefined;
    case 'json':
      return value;
    default:
      return undefined;
  }
}

export function defaultValue(field) {
  if (field.default !== undefined) return field.default;
  switch (field.type) {
    case 'text':
    case 'slug':
    case 'long_text':
    case 'rich_text':
    case 'media':
    case 'reference':
      return '';
    case 'number':
    case 'integer':
      return 0;
    case 'boolean':
      return false;
    case 'json':
      return null;
    case 'enum':
      return (field.choices ?? [])[0] ?? '';
    default:
      return null;
  }
}

export function slugField(name = 'slug') {
  return { name, type: 'slug', required: true, unique: true, max: 90 };
}

export function timelineFields() {
  return [
    { name: 'created_at', type: 'date', required: false, system: true },
    { name: 'updated_at', type: 'date', required: false, system: true },
    { name: 'published_at', type: 'date', required: false, system: true },
  ];
}

export function systemField(name, type) {
  return { name, type, system: true, required: false };
}

export function slugFromValues(fields, values) {
  const slugFieldDef = fields.find((field) => field.type === 'slug');
  const titleField = fields.find((field) => field.name === 'title') ?? fields.find((field) => field.type === 'text');
  if (!slugFieldDef) return values.slug;
  if (values.slug) return slugify(values.slug);
  return slugify(values[titleField?.name] ?? '');
}

export function fieldNames(schema) {
  return (schema.fields ?? []).map((field) => field.name);
}

export function requiredFields(schema) {
  return (schema.fields ?? []).filter((field) => field.required).map((field) => field.name);
}

export function findField(schema, name) {
  return (schema.fields ?? []).find((field) => field.name === name);
}

export function normalizeField(field) {
  if (!isFieldType(field.type)) throw new TypeError(`field ${field.name} has an unknown type ${field.type}`);
  if (typeof field.name !== 'string' || !/^[a-z][a-z0-9_]{0,40}$/.test(field.name)) {
    throw new TypeError(`field name ${JSON.stringify(field.name)} is not a snake_case identifier`);
  }
  const out = { ...field };
  if (out.type === 'enum') {
    if (!Array.isArray(out.choices) || out.choices.length === 0) {
      throw new TypeError(`enum field ${out.name} needs choices`);
    }
    out.choices = out.choices.map((choice) => singleLine(choice)).filter(Boolean);
  }
  if (out.required === undefined) out.required = false;
  return out;
}

export function cloneField(field) {
  return { ...field, choices: field.choices ? [...field.choices] : undefined };
}

export function fieldDiff(before, after) {
  const changes = [];
  if (before.type !== after.type) changes.push({ key: 'type', before: before.type, after: after.type });
  if (Boolean(before.required) !== Boolean(after.required)) {
    changes.push({ key: 'required', before: Boolean(before.required), after: Boolean(after.required) });
  }
  if ((before.max ?? null) !== (after.max ?? null)) changes.push({ key: 'max', before: before.max ?? null, after: after.max ?? null });
  return changes;
}

export function emptyValue(field) {
  return defaultValue(field);
}

export function isRequiredLike(field) {
  return field.required === true && !field.system;
}

export function fieldCategories(schema) {
  const out = new Map();
  for (const field of schema.fields ?? []) {
    const category = field.type === 'media' ? 'assets' : field.type === 'reference' ? 'links' : 'values';
    const bucket = out.get(category);
    if (bucket) bucket.push(field.name);
    else out.set(category, [field.name]);
  }
  return out;
}

export function coerceAll(schema, values) {
  const out = {};
  for (const field of schema.fields ?? []) {
    if (field.system === true) continue;
    if (values[field.name] === undefined) continue;
    const coerced = coerceValue(field, values[field.name]);
    if (coerced !== undefined) out[field.name] = coerced;
  }
  return out;
}

export function describeField(field) {
  const parts = [field.name, `(${field.type})`];
  if (field.required) parts.push('required');
  if (field.unique) parts.push('unique');
  if (field.max) parts.push(`max ${field.max}`);
  return parts.join(' ');
}

export function areFieldsCompatible(left, right) {
  return left.name === right.name && left.type === right.type;
}
