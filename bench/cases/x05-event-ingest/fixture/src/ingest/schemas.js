import { badRequest } from '../util/errors.js';
import { isBlank, singleLine } from '../util/text.js';

export const EVENT_KINDS = ['page_view', 'click', 'purchase', 'signup', 'custom'];

export function defineEventType(input = {}) {
  const name = singleLine(input.name ?? '');
  if (!/^[a-z][a-z0-9_]{1,40}$/.test(name)) throw badRequest('invalid_event_type', 'an event type name is snake_case');
  const fields = Array.isArray(input.fields) ? input.fields : [];
  for (const field of fields) {
    if (!/^[a-z][a-z0-9_]{0,40}$/.test(String(field.name ?? ''))) {
      throw badRequest('invalid_field', `field ${field.name} is not snake_case`);
    }
    if (!['string', 'number', 'boolean', 'timestamp'].includes(field.type)) {
      throw badRequest('invalid_field', `field ${field.name} has an unknown type ${field.type}`);
    }
  }
  return {
    name,
    kind: EVENT_KINDS.includes(input.kind) ? input.kind : 'custom',
    fields,
    required: fields.filter((field) => field.required === true).map((field) => field.name),
    created_at: input.created_at,
  };
}

export class SchemaBook {
  constructor(types = []) {
    this.types = new Map();
    for (const type of types) this.types.set(type.name, type);
  }

  define(input) {
    const type = defineEventType(input);
    if (this.types.has(type.name)) throw badRequest('event_type_exists', `event type ${type.name} exists`);
    this.types.set(type.name, type);
    return type;
  }

  get(name) {
    return this.types.get(String(name));
  }

  list() {
    return [...this.types.values()].map((type) => ({
      name: type.name,
      kind: type.kind,
      fields: type.fields.length,
      required: type.required.length,
    }));
  }

  remove(name) {
    return this.types.delete(String(name));
  }

  size() {
    return this.types.size;
  }
}

export function validateEvent(type, event = {}) {
  const problems = [];
  if (!event.name || isBlank(event.name)) problems.push('event name is required');
  if (type) {
    if (event.name !== type.name && !EVENT_KINDS.includes(String(event.name))) {
      problems.push(`${event.name} is not a declared event type`);
    }
    for (const field of type.fields) {
      const value = event.properties?.[field.name];
      if (field.required === true && (value === undefined || value === null || value === '')) {
        problems.push(`${field.name} is required`);
        continue;
      }
      if (value === undefined) continue;
      if (field.type === 'number' && !Number.isFinite(Number(value))) problems.push(`${field.name} is not a number`);
      if (field.type === 'boolean' && typeof value !== 'boolean') problems.push(`${field.name} is not a boolean`);
      if (field.type === 'timestamp' && Number.isNaN(new Date(String(value)).getTime())) {
        problems.push(`${field.name} is not a timestamp`);
      }
      if (field.type === 'string' && String(value).length > (field.max ?? 500)) {
        problems.push(`${field.name} is longer than ${field.max ?? 500} characters`);
      }
    }
  }
  const samples = event.samples;
  if (samples !== undefined) {
    if (!Array.isArray(samples)) problems.push('samples must be a list');
    else if (samples.some((sample) => !Number.isFinite(Number(sample)))) problems.push('samples must be numbers');
  }
  return { ok: problems.length === 0, problems };
}

export function defaultTypes() {
  return [
    {
      name: 'page_view',
      kind: 'page_view',
      fields: [
        { name: 'path', type: 'string', required: true, max: 300 },
        { name: 'referrer', type: 'string', max: 500 },
        { name: 'duration_ms', type: 'number' },
      ],
      required: ['path'],
    },
    {
      name: 'purchase',
      kind: 'purchase',
      fields: [
        { name: 'order_id', type: 'string', required: true, max: 60 },
        { name: 'amount_cents', type: 'number', required: true },
        { name: 'currency', type: 'string', max: 3 },
      ],
      required: ['order_id', 'amount_cents'],
    },
  ];
}
