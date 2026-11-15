export class ValidateError extends Error {
  constructor(message, field) {
    super(message);
    this.name = 'ValidateError';
    this.code = 'invalid_field';
    this.status = 400;
    this.field = field;
  }

  toJSON() {
    return { error: this.code, message: this.message, field: this.field };
  }
}

export function assertShape(value, shape, label = 'value') {
  if (shape === 'object' && (value === null || typeof value !== 'object' || Array.isArray(value))) {
    throw new ValidateError(`${label} must be an object`, label);
  }
  if (shape === 'array' && !Array.isArray(value)) throw new ValidateError(`${label} must be a list`, label);
  if (shape === 'string' && typeof value !== 'string') throw new ValidateError(`${label} must be text`, label);
  return true;
}

export function requireString(value, label, options = {}) {
  const text = typeof value === 'string' ? value.trim() : '';
  if (text === '') throw new ValidateError(`${label} is required`, label);
  if (options.max !== undefined && text.length > options.max) {
    throw new ValidateError(`${label} is longer than ${options.max} characters`, label);
  }
  if (options.pattern !== undefined && !options.pattern.test(text)) {
    throw new ValidateError(`${label} is not in the expected shape`, label);
  }
  return text;
}

export function optionalString(value, label, options = {}) {
  if (value === undefined || value === null || String(value).trim() === '') return options.fallback;
  return requireString(value, label, options);
}

export function requireEnum(value, allowed, label) {
  const text = String(value ?? '');
  if (!allowed.includes(text)) throw new ValidateError(`${label} must be one of ${allowed.join(', ')}`, label);
  return text;
}

export function requireArray(value, label) {
  if (!Array.isArray(value)) throw new ValidateError(`${label} must be a list`, label);
  return value;
}

export function requireInteger(value, label, options = {}) {
  const number = Number(value);
  if (!Number.isInteger(number)) throw new ValidateError(`${label} must be a whole number`, label);
  if (options.min !== undefined && number < options.min) throw new ValidateError(`${label} must be at least ${options.min}`, label);
  if (options.max !== undefined && number > options.max) throw new ValidateError(`${label} must be at most ${options.max}`, label);
  return number;
}

export function clampInt(value, min, max, fallback) {
  const number = Number(value);
  if (!Number.isFinite(number)) return fallback;
  return Math.min(Math.max(Math.trunc(number), min), max);
}

export function assertNoUnsafeKeys(value, label = 'value') {
  if (value === null || typeof value !== 'object') return true;
  for (const [name, item] of Object.entries(value)) {
    if (['__proto__', 'constructor', 'prototype'].includes(name)) {
      throw new ValidateError(`${label} carries an unsafe key ${name}`, label);
    }
    assertNoUnsafeKeys(item, label);
  }
  return true;
}

export function collectProblems(value, shape = {}) {
  const problems = [];
  for (const [field, rules] of Object.entries(shape)) {
    const item = value?.[field];
    if (rules.required === true && (item === undefined || item === null || String(item).trim() === '')) {
      problems.push({ field, message: `${field} is required` });
    }
  }
  return problems;
}

export function checkShape(value, shape, label = 'body') {
  const problems = [];
  for (const [field, rules] of Object.entries(shape ?? {})) {
    const item = value?.[field];
    if (rules.required === true && (item === undefined || item === null || String(item).trim() === '')) {
      problems.push({ field, message: `${field} is required` });
      continue;
    }
    if (item === undefined) continue;
    if (rules.type === 'string' && typeof item !== 'string') problems.push({ field, message: `${field} must be text` });
    if (rules.type === 'number' && !Number.isFinite(Number(item))) problems.push({ field, message: `${field} must be a number` });
    if (rules.type === 'array' && !Array.isArray(item)) problems.push({ field, message: `${field} must be a list` });
    if (rules.enum !== undefined && !rules.enum.includes(item)) problems.push({ field, message: `${field} must be one of ${rules.enum.join(', ')}` });
  }
  return { ok: problems.length === 0, problems, label };
}

export function assertShapeOf(value, shape, label) {
  const result = checkShape(value, shape, label);
  if (!result.ok) throw new ValidateError(result.problems[0].message, result.problems[0].field);
  return true;
}

export function asInteger(value, fallback = 0) {
  const number = Number(value);
  return Number.isInteger(number) ? number : fallback;
}

export function asBoolean(value, fallback = false) {
  if (value === true || value === 'true') return true;
  if (value === false || value === 'false') return false;
  return fallback;
}
