import { invalidField } from '../util/errors.js';
import { singleLine } from '../util/text.js';

export const HOOK_NAMES = [
  'entry.beforeCreate',
  'entry.afterCreate',
  'entry.afterUpdate',
  'entry.beforePublish',
  'entry.afterPublish',
  'media.afterUpload',
  'render.beforeFragment',
  'render.afterDocument',
  'export.afterWrite',
];

const NAME_PATTERN = /^[a-z][a-z0-9-]{1,40}$/;
const VERSION_PATTERN = /^\d+\.\d+\.\d+$/;

export function parseManifest(raw, options = {}) {
  let input = raw;
  if (typeof raw === 'string') {
    try {
      input = JSON.parse(raw);
    } catch (error) {
      throw invalidField('manifest', `is not valid JSON: ${error.message}`);
    }
  }
  if (input === null || typeof input !== 'object' || Array.isArray(input)) {
    throw invalidField('manifest', 'must be an object');
  }
  const name = singleLine(input.name ?? '');
  if (!NAME_PATTERN.test(name)) throw invalidField('manifest.name', 'must be a lowercase slug');
  const version = singleLine(input.version ?? '');
  if (!VERSION_PATTERN.test(version)) throw invalidField('manifest.version', 'must be semver');
  const hooks = Array.isArray(input.hooks) ? input.hooks : [];
  for (const hook of hooks) {
    if (!HOOK_NAMES.includes(hook)) throw invalidField('manifest.hooks', `names unknown hook ${hook}`);
  }
  const permissions = Array.isArray(input.permissions) ? input.permissions : [];
  const enabled = input.enabled === undefined ? true : input.enabled === true;
  return {
    name,
    version,
    title: singleLine(input.title ?? name),
    description: singleLine(input.description ?? ''),
    author: singleLine(input.author ?? ''),
    hooks: [...new Set(hooks)],
    permissions: permissions.map((entry) => singleLine(entry)).filter(Boolean),
    format: singleLine(input.format ?? 'fragment'),
    template: singleLine(input.template ?? 'entry'),
    enabled,
    entry: singleLine(input.entry ?? 'index.js'),
    root: options.root,
  };
}

export function manifestSummary(manifest) {
  return {
    name: manifest.name,
    version: manifest.version,
    title: manifest.title,
    hooks: manifest.hooks.length,
    enabled: manifest.enabled,
  };
}

export function validateManifestUpdate(manifest, patch = {}) {
  const problems = [];
  if (patch.version !== undefined && !VERSION_PATTERN.test(singleLine(patch.version))) {
    problems.push('version must be semver');
  }
  if (patch.hooks !== undefined) {
    if (!Array.isArray(patch.hooks)) problems.push('hooks must be a list');
    else {
      for (const hook of patch.hooks) {
        if (!HOOK_NAMES.includes(hook)) problems.push(`unknown hook ${hook}`);
      }
    }
  }
  if (problems.length > 0) throw invalidField('manifest', problems.join('; '));
  return true;
}

export function compareVersions(left, right) {
  const a = String(left).split('.').map(Number);
  const b = String(right).split('.').map(Number);
  for (let index = 0; index < 3; index += 1) {
    if ((a[index] ?? 0) !== (b[index] ?? 0)) return (a[index] ?? 0) > (b[index] ?? 0) ? 1 : -1;
  }
  return 0;
}

export function requiresPermission(manifest, permission) {
  return (manifest.permissions ?? []).includes(permission);
}
