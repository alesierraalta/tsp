import { conflict, notFound } from '../util/errors.js';
import { compareVersions, manifestSummary } from './manifest.js';

export class PluginRegistry {
  constructor() {
    this.plugins = new Map();
    this.order = [];
  }

  register(manifest) {
    if (this.plugins.has(manifest.name)) {
      throw conflict('plugin_exists', `plugin ${manifest.name} is already registered`);
    }
    this.plugins.set(manifest.name, manifest);
    this.order.push(manifest.name);
    return manifest;
  }

  replace(manifest) {
    if (!this.plugins.has(manifest.name)) throw notFound(`plugin ${manifest.name}`);
    this.plugins.set(manifest.name, manifest);
    return manifest;
  }

  get(name) {
    const found = this.plugins.get(String(name));
    if (!found) throw notFound(`plugin ${name}`);
    return found;
  }

  has(name) {
    return this.plugins.has(String(name));
  }

  remove(name) {
    const existed = this.plugins.delete(String(name));
    if (existed) this.order = this.order.filter((entry) => entry !== name);
    return existed;
  }

  list() {
    return this.order.map((name) => manifestSummary(this.plugins.get(name)));
  }

  all() {
    return this.order.map((name) => this.plugins.get(name));
  }

  enabled() {
    return this.all().filter((manifest) => manifest.enabled);
  }

  forHook(hook) {
    return this.enabled().filter((manifest) => manifest.hooks.includes(hook));
  }

  bump(name, version) {
    const manifest = this.get(name);
    if (compareVersions(version, manifest.version) <= 0) {
      throw conflict('version_regression', `${version} does not advance ${manifest.version}`);
    }
    return this.replace({ ...manifest, version });
  }

  stats() {
    return {
      total: this.plugins.size,
      enabled: this.enabled().length,
      hooks: this.enabled().reduce((total, manifest) => total + manifest.hooks.length, 0),
    };
  }
}

export function loadRegistry(manifests) {
  const registry = new PluginRegistry();
  for (const manifest of manifests) registry.register(manifest);
  return registry;
}
