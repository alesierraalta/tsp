import { invalidField, notFound } from '../http/errors.js';

export const BLOCKED_KEYS = new Set(['__proto__', 'constructor', 'prototype']);

const TENANT_FIELDS = {
  locale: { type: 'string', pattern: /^[a-z]{2}(-[A-Z]{2})?$/, fallback: 'en' },
  retention_days: { type: 'integer', min: 1, max: 3650, fallback: 30 },
  digest: { type: 'string', choices: ['daily', 'weekly', 'never'], fallback: 'weekly' },
  features: {
    type: 'object',
    fields: {
      audit_log: { type: 'boolean', fallback: true },
      exports: { type: 'boolean', fallback: true },
      api_tokens: { type: 'boolean', fallback: false },
    },
  },
  notifications: {
    type: 'object',
    fields: {
      email: { type: 'boolean', fallback: true },
      webhook_url: { type: 'string', pattern: /^https:\/\/[a-z0-9.-]+(\/[^\s]*)?$/i, fallback: '' },
    },
  },
};

const USER_FIELDS = {
  theme: { type: 'string', choices: ['light', 'dark', 'system'], fallback: 'system' },
  density: { type: 'string', choices: ['comfortable', 'compact'], fallback: 'comfortable' },
  timezone: { type: 'string', pattern: /^[A-Za-z]+\/[A-Za-z_]+$/, fallback: 'UTC' },
  digest: { type: 'string', choices: ['daily', 'weekly', 'never'], fallback: 'weekly' },
  shortcuts: { type: 'boolean', fallback: true },
};

export const SETTINGS_SCHEMA = {
  tenant: TENANT_FIELDS,
  user: USER_FIELDS,
};

function schemaOf(kind) {
  const schema = SETTINGS_SCHEMA[kind];
  if (!schema) throw new TypeError(`unknown settings kind: ${kind}`);
  return schema;
}

function checkType(rule, value) {
  switch (rule.type) {
    case 'string':
      if (typeof value !== 'string') return 'must be a string';
      if (rule.pattern && !rule.pattern.test(value)) return 'has an unsupported format';
      if (rule.choices && !rule.choices.includes(value)) {
        return `must be one of ${rule.choices.join(', ')}`;
      }
      return undefined;
    case 'boolean':
      if (typeof value !== 'boolean') return 'must be true or false';
      return undefined;
    case 'integer':
      if (!Number.isInteger(value)) return 'must be a whole number';
      if (rule.min !== undefined && value < rule.min) return `must be at least ${rule.min}`;
      if (rule.max !== undefined && value > rule.max) return `must be at most ${rule.max}`;
      return undefined;
    case 'object':
      if (value === null || typeof value !== 'object' || Array.isArray(value)) {
        return 'must be an object';
      }
      return undefined;
    default:
      return 'has no validator';
  }
}

export function defaultsFor(kind) {
  const schema = schemaOf(kind);
  const out = {};
  for (const [field, rule] of Object.entries(schema)) {
    if (rule.type === 'object') {
      out[field] = defaultsOfFields(rule.fields);
      continue;
    }
    out[field] = rule.fallback;
  }
  return out;
}

function defaultsOfFields(fields) {
  const out = {};
  for (const [field, rule] of Object.entries(fields)) {
    if (rule.type === 'object') {
      out[field] = defaultsOfFields(rule.fields);
      continue;
    }
    out[field] = rule.fallback;
  }
  return out;
}

export function defaultsFrom(fields) {
  return defaultsOfFields(fields);
}

export function mergeSettings(kind, base, patch) {
  const schema = schemaOf(kind);
  if (patch === null || typeof patch !== 'object' || Array.isArray(patch)) {
    throw new TypeError('a settings patch must be an object');
  }
  const out = { ...defaultsFor(kind) };
  for (const [key, value] of Object.entries(base ?? {})) {
    if (BLOCKED_KEYS.has(key) || !(key in schema)) continue;
    out[key] = value;
  }
  for (const key of Object.keys(patch)) {
    if (BLOCKED_KEYS.has(key)) {
      throw invalidField(key, 'is not a setting');
    }
    const rule = schema[key];
    if (!rule) {
      throw invalidField(key, 'is not a setting');
    }
    const value = patch[key];
    if (rule.type === 'object') {
      out[key] = mergeNested(key, rule.fields, out[key] ?? defaultsFrom(rule.fields), value);
      continue;
    }
    const problem = checkType(rule, value);
    if (problem) {
      throw invalidField(key, problem);
    }
    out[key] = value;
  }
  return out;
}

function mergeNested(prefix, fields, base, patch) {
  if (patch === null || typeof patch !== 'object' || Array.isArray(patch)) {
    throw invalidField(prefix, 'must be an object');
  }
  const out = { ...base };
  for (const key of Object.keys(patch)) {
    if (BLOCKED_KEYS.has(key)) {
      throw invalidField(`${prefix}.${key}`, 'is not a setting');
    }
    const rule = fields[key];
    if (!rule) {
      throw invalidField(`${prefix}.${key}`, 'is not a setting');
    }
    const value = patch[key];
    if (rule.type === 'object') {
      out[key] = mergeNested(`${prefix}.${key}`, rule.fields, out[key] ?? defaultsOfFields(rule.fields), value);
      continue;
    }
    const problem = checkType(rule, value);
    if (problem) {
      throw invalidField(`${prefix}.${key}`, problem);
    }
    out[key] = value;
  }
  return out;
}

export class SettingsService {
  constructor(options = {}) {
    this.store = options.store;
    if (!this.store) throw new TypeError('a settings store is required');
    this.clock = options.clock ?? (() => new Date());
  }

  tenantOf(tenantId) {
    const tenant = this.store.get('tenants', tenantId);
    if (!tenant) throw notFound(`tenant ${tenantId}`);
    return tenant;
  }

  userOf(userId) {
    const user = this.store.get('users', userId);
    if (!user) throw notFound(`user ${userId}`);
    return user;
  }

  getTenant(tenantId) {
    return { ...this.tenantOf(tenantId).settings };
  }

  updateTenant(tenantId, patch) {
    const tenant = this.tenantOf(tenantId);
    const next = mergeSettings('tenant', tenant.settings, patch);
    this.store.update('tenants', tenantId, (row) => {
      row.settings = next;
      return row;
    });
    return { ...next };
  }

  getUser(userId) {
    return { ...this.userOf(userId).settings };
  }

  updateUser(userId, patch) {
    const user = this.userOf(userId);
    const next = mergeSettings('user', user.settings, patch);
    this.store.update('users', userId, (row) => {
      row.settings = next;
      return row;
    });
    return { ...next };
  }

  resetTenant(tenantId) {
    this.tenantOf(tenantId);
    const next = defaultsFor('tenant');
    this.store.update('tenants', tenantId, (row) => {
      row.settings = next;
      return row;
    });
    return { ...next };
  }

  resetUser(userId) {
    this.userOf(userId);
    const next = defaultsFor('user');
    this.store.update('users', userId, (row) => {
      row.settings = next;
      return row;
    });
    return { ...next };
  }

  describe(kind) {
    return Object.keys(schemaOf(kind)).sort();
  }
}
