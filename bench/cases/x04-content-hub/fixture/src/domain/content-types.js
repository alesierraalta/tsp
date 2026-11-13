import { kebabCase, singleLine } from '../util/text.js';
import { invalidField } from '../util/errors.js';
import { findField, normalizeField, requiredFields, slugField, timelineFields } from './fields.js';

const RESERVED = new Set(['id', 'type', 'locale', 'status', 'version', 'created_at', 'updated_at']);

export function normalizeApiId(input) {
  const text = kebabCase(singleLine(input ?? ''));
  if (!/^[a-z][a-z0-9-]{0,50}$/.test(text)) {
    throw invalidField('api_id', 'must be a kebab-case identifier');
  }
  return text;
}

export function defineContentType(input = {}) {
  const apiId = normalizeApiId(input.api_id ?? input.name);
  const fields = (input.fields ?? []).map((field) => normalizeField(field));
  if (fields.length === 0) throw invalidField('fields', 'must declare at least one field');
  const names = new Set();
  for (const field of fields) {
    if (RESERVED.has(field.name)) throw invalidField('fields', `may not redeclare ${field.name}`);
    if (names.has(field.name)) throw invalidField('fields', `declares ${field.name} twice`);
    names.add(field.name);
  }
  if (!fields.some((field) => field.type === 'slug')) fields.push(normalizeField(slugField()));
  return {
    id: input.id ?? `ct_${apiId}`,
    api_id: apiId,
    name: singleLine(input.name ?? apiId),
    description: singleLine(input.description ?? ''),
    fields: [...fields, ...timelineFields().filter((field) => !names.has(field.name))],
    i18n: input.i18n === true,
    draft_and_publish: input.draft_and_publish !== false,
    created_at: input.created_at,
  };
}

export function contentTypeView(schema) {
  return {
    api_id: schema.api_id,
    name: schema.name,
    description: schema.description,
    i18n: schema.i18n,
    draft_and_publish: schema.draft_and_publish,
    fields: schema.fields.filter((field) => field.system !== true).map(publicField),
  };
}

function publicField(field) {
  const out = { name: field.name, type: field.type, required: field.required === true };
  if (field.choices) out.choices = [...field.choices];
  if (field.unique) out.unique = true;
  if (field.max) out.max = field.max;
  return out;
}

export function validateContentTypePatch(schema, patch = {}) {
  const problems = [];
  if (patch.name !== undefined && singleLine(patch.name) === '') problems.push('name may not be blank');
  if (patch.fields !== undefined) {
    if (!Array.isArray(patch.fields)) problems.push('fields must be a list');
    else {
      const next = new Set();
      for (const field of patch.fields) {
        const name = String(field?.name ?? '');
        if (RESERVED.has(name)) problems.push(`field ${name} is reserved`);
        if (next.has(name)) problems.push(`field ${name} is declared twice`);
        next.add(name);
      }
    }
  }
  if (problems.length > 0) throw invalidField('patch', problems.join('; '));
  return true;
}

export function applyContentTypePatch(schema, patch = {}) {
  validateContentTypePatch(schema, patch);
  const next = { ...schema };
  if (patch.name !== undefined) next.name = singleLine(patch.name);
  if (patch.description !== undefined) next.description = singleLine(patch.description);
  if (patch.fields !== undefined) {
    const kept = schema.fields.filter((field) => field.system === true);
    const declared = [...kept];
    for (const field of patch.fields) {
      const existing = findField(schema, field.name);
      declared.push(normalizeField({ ...(existing ?? {}), ...field }));
    }
    next.fields = declared;
  }
  for (const flag of ['i18n', 'draft_and_publish']) {
    if (patch[flag] !== undefined) next[flag] = patch[flag] === true;
  }
  return next;
}

export function editableFields(schema) {
  return (schema.fields ?? []).filter((field) => field.system !== true);
}

export function searchableFields(schema) {
  return editableFields(schema)
    .filter((field) => ['text', 'long_text', 'rich_text', 'slug', 'enum'].includes(field.type))
    .map((field) => field.name);
}

export function referenceFields(schema) {
  return editableFields(schema).filter((field) => field.type === 'reference');
}

export function mediaFields(schema) {
  return editableFields(schema).filter((field) => field.type === 'media');
}

export function contentTypeSummary(schema) {
  return {
    api_id: schema.api_id,
    name: schema.name,
    fields: editableFields(schema).length,
    i18n: schema.i18n === true,
  };
}

export function diffSchemas(before, after) {
  const beforeNames = new Set(before.fields.map((field) => field.name));
  const afterNames = new Set(after.fields.map((field) => field.name));
  const added = [...afterNames].filter((name) => !beforeNames.has(name));
  const removed = [...beforeNames].filter((name) => !afterNames.has(name));
  const changed = [...afterNames].filter((name) => {
    if (!beforeNames.has(name)) return false;
    const left = findField(before, name);
    const right = findField(after, name);
    return left.type !== right.type || Boolean(left.required) !== Boolean(right.required);
  });
  return { added, removed, changed, destructive: removed.length > 0 };
}

export function fieldUsage(schema, entries) {
  const counts = {};
  for (const field of editableFields(schema)) {
    counts[field.name] = entries.filter((entry) => {
      const value = entry.values?.[field.name];
      return value !== undefined && value !== null && String(value).trim() !== '';
    }).length;
  }
  return counts;
}

export function migrateValues(schema, values, rename) {
  const out = {};
  for (const [key, value] of Object.entries(values ?? {})) {
    const target = rename[key] ?? key;
    if (findField(schema, target)) out[target] = value;
  }
  return out;
}

export function assertCompatiblePatch(before, after, entries) {
  const diff = diffSchemas(before, after);
  if (diff.removed.length === 0) return diff;
  const used = diff.removed.filter((name) => entries.some((entry) => entry.values?.[name] !== undefined));
  if (used.length > 0) {
    throw new TypeError(`fields ${used.join(', ')} still hold values and cannot be removed`);
  }
  return diff;
}

export function sortFields(fields) {
  return [...fields].sort((left, right) => {
    if (Boolean(left.system) !== Boolean(right.system)) return left.system ? 1 : -1;
    if (Boolean(left.required) !== Boolean(right.required)) return left.required ? -1 : 1;
    return left.name < right.name ? -1 : 1;
  });
}

export function schemaChecklist(schema) {
  const fields = editableFields(schema);
  return {
    has_slug: schema.fields.some((field) => field.type === 'slug'),
    has_title: fields.some((field) => field.name === 'title'),
    required: requiredFields(schema).length,
    media: mediaFields(schema).length,
    references: referenceFields(schema).length,
  };
}
