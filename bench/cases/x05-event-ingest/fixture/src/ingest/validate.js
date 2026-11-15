import { badRequest } from '../util/errors.js';
import { ValidateError } from '../util/validate.js';

export const FIELD_TYPES = ['string', 'number', 'boolean', 'timestamp', 'list'];

export function checkField(field, value) {
  if (value === undefined || value === null) {
    return field.required === true ? `${field.name} is required` : undefined;
  }
  switch (field.type) {
    case 'string':
      if (typeof value !== 'string') return `${field.name} is not a string`;
      if (value.length > (field.max ?? 500)) return `${field.name} is longer than ${field.max ?? 500} characters`;
      return undefined;
    case 'number':
      if (!Number.isFinite(Number(value))) return `${field.name} is not a number`;
      if (field.min !== undefined && Number(value) < field.min) return `${field.name} is below ${field.min}`;
      if (field.max !== undefined && Number(value) > field.max) return `${field.name} is above ${field.max}`;
      return undefined;
    case 'boolean':
      return typeof value === 'boolean' ? undefined : `${field.name} is not a boolean`;
    case 'timestamp':
      return Number.isNaN(new Date(String(value)).getTime()) ? `${field.name} is not a timestamp` : undefined;
    case 'list':
      return Array.isArray(value) ? undefined : `${field.name} is not a list`;
    default:
      return `${field.name} has an unknown type ${field.type}`;
  }
}

export function validateProperties(type, properties = {}) {
  const problems = [];
  const declared = new Set((type?.fields ?? []).map((field) => field.name));
  for (const field of type?.fields ?? []) {
    const problem = checkField(field, properties[field.name]);
    if (problem) problems.push({ field: field.name, message: problem });
  }
  for (const key of Object.keys(properties)) {
    if (!declared.has(key)) problems.push({ field: key, message: `${key} is not declared by ${type?.name ?? 'the event'}` });
  }
  return problems;
}

export function assertProperties(type, properties) {
  const problems = validateProperties(type, properties);
  if (problems.length > 0) {
    throw badRequest('invalid_properties', problems[0].message, { problems });
  }
  return true;
}

export function coerceProperties(type, properties = {}) {
  const out = { ...properties };
  for (const field of type?.fields ?? []) {
    const value = out[field.name];
    if (value === undefined) continue;
    if (field.type === 'number' && typeof value === 'string' && value.trim() !== '') {
      const number = Number(value);
      if (Number.isFinite(number)) out[field.name] = number;
    }
    if (field.type === 'boolean' && typeof value === 'string') {
      if (value === 'true') out[field.name] = true;
      if (value === 'false') out[field.name] = false;
    }
    if (field.type === 'timestamp' && typeof value === 'string') {
      const parsed = new Date(value);
      if (!Number.isNaN(parsed.getTime())) out[field.name] = parsed.toISOString();
    }
  }
  return out;
}

export function normalizeField(field) {
  if (!FIELD_TYPES.includes(field.type)) throw new ValidateError(`field ${field.name} has type ${field.type}`);
  return {
    name: String(field.name),
    type: field.type,
    required: field.required === true,
    min: field.min,
    max: field.max,
  };
}

export function errorSummary(problems) {
  if (problems.length === 0) return 'no problems';
  return `${problems.length} problem(s): ${problems.map((problem) => problem.field).join(', ')}`;
}

export function validateBatch(type, events) {
  const accepted = [];
  const rejected = [];
  for (const [index, event] of (events ?? []).entries()) {
    const problems = validateProperties(type, event?.properties ?? {});
    if (problems.length === 0) accepted.push(index);
    else rejected.push({ index, problems });
  }
  return { accepted, rejected, rate: events?.length ? Math.round((rejected.length / events.length) * 10000) / 100 : 0 };
}

export function missingRequired(type, properties = {}) {
  return (type?.fields ?? [])
    .filter((field) => field.required === true)
    .filter((field) => properties[field.name] === undefined || properties[field.name] === null || properties[field.name] === '')
    .map((field) => field.name);
}

export function unknownFields(type, properties = {}) {
  const declared = new Set((type?.fields ?? []).map((field) => field.name));
  return Object.keys(properties).filter((name) => !declared.has(name));
}

export function fieldSummary(type) {
  const fields = type?.fields ?? [];
  return {
    name: type?.name,
    fields: fields.length,
    required: fields.filter((field) => field.required === true).length,
    types: [...new Set(fields.map((field) => field.type))].sort(),
    widest: fields.reduce((best, field) => (best === undefined || (field.max ?? 0) > (best.max ?? 0) ? field : best), undefined)?.name,
  };
}

export function checkAll(type, events) {
  return (events ?? []).map((event) => ({
    ok: validateProperties(type, event?.properties ?? {}).length === 0,
    missing: missingRequired(type, event?.properties ?? {}),
    unknown: unknownFields(type, event?.properties ?? {}),
  }));
}
