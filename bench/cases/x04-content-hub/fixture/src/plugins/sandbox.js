import { forbidden } from '../util/errors.js';
import { requiresPermission } from './manifest.js';

export const CAPABILITIES = {
  'content:read': 'read entries through hooks',
  'content:write': 'modify entries through hooks',
  'media:read': 'read media metadata',
  'media:write': 'write media derivatives',
  'render:template': 'add template helpers',
  'render:process': 'spawn the render tool',
  'export:write': 'write export files',
};

export const HOOK_CAPABILITIES = {
  'entry.beforeCreate': ['content:write'],
  'entry.afterCreate': ['content:read'],
  'entry.afterUpdate': ['content:read'],
  'entry.beforePublish': ['content:write'],
  'entry.afterPublish': ['render:process'],
  'media.afterUpload': ['media:write'],
  'render.beforeFragment': ['render:template'],
  'render.afterDocument': ['render:template'],
  'export.afterWrite': ['export:write'],
};

export function requiredCapabilities(hook) {
  return HOOK_CAPABILITIES[hook] ?? [];
}

export function missingCapabilities(manifest) {
  const needed = new Set();
  for (const hook of manifest.hooks ?? []) {
    for (const capability of requiredCapabilities(hook)) needed.add(capability);
  }
  return [...needed].filter((capability) => !requiresPermission(manifest, capability));
}

export function assertCapabilities(manifest) {
  const missing = missingCapabilities(manifest);
  if (missing.length > 0) {
    throw forbidden(`plugin ${manifest.name} needs ${missing.join(', ')}`);
  }
  return true;
}

export function capabilityReport(manifest) {
  const granted = new Set(manifest.permissions ?? []);
  return Object.entries(CAPABILITIES).map(([capability, description]) => ({
    capability,
    description,
    granted: granted.has(capability),
    needed: (manifest.hooks ?? []).some((hook) => requiredCapabilities(hook).includes(capability)),
  }));
}

export class Sandbox {
  constructor(options = {}) {
    this.manifests = options.registry;
    this.calls = [];
  }

  check(pluginName, capability) {
    const manifest = this.manifests.get(pluginName);
    const granted = (manifest.permissions ?? []).includes(capability);
    this.calls.push({ plugin: pluginName, capability, granted });
    if (!granted) throw forbidden(`plugin ${pluginName} may not ${capability}`);
    return true;
  }

  can(pluginName, capability) {
    try {
      return this.check(pluginName, capability);
    } catch {
      return false;
    }
  }

  denied() {
    return this.calls.filter((call) => !call.granted);
  }

  stats() {
    return {
      checks: this.calls.length,
      denied: this.denied().length,
      plugins: this.manifests.all().length,
    };
  }
}

export function describeCapability(capability) {
  return CAPABILITIES[capability] ?? 'unknown capability';
}
