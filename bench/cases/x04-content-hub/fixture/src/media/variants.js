import { createHash } from 'node:crypto';
import { assertPlainName, extensionOf, baseName, variantName } from './paths.js';
import { aspectRatio, PRESETS, scaleTo } from './images.js';

export function fingerprint(buffer) {
  return createHash('sha256').update(buffer).digest('hex').slice(0, 16);
}

export function variantKey(name, variant, sourceVersion) {
  return `${baseName(assertPlainName(name))}:${variant}:${sourceVersion}`;
}

export function planVariants(name, size, wanted) {
  const presets = wanted && wanted.length > 0 ? wanted : Object.keys(PRESETS);
  const effective = presets.filter((preset) => Object.prototype.hasOwnProperty.call(PRESETS, preset));
  return effective.map((preset) => {
    const limit = PRESETS[preset];
    const scaled = scaleTo(size, limit);
    return {
      preset,
      name: variantName(name, preset),
      width: scaled?.width ?? limit.width,
      height: scaled?.height ?? limit.height,
      scaled: scaled?.scaled ?? false,
    };
  });
}

export class VariantCache {
  constructor(options = {}) {
    this.render = options.render;
    this.entries = new Map();
    this.hits = 0;
    this.misses = 0;
  }

  async get(name, preset, sourceVersion) {
    const key = variantKey(name, preset, sourceVersion);
    const hit = this.entries.get(key);
    if (hit) {
      this.hits += 1;
      return hit;
    }
    this.misses += 1;
    if (typeof this.render !== 'function') throw new TypeError('a variant cache needs a renderer');
    const value = await this.render({ name, preset, sourceVersion, key });
    this.entries.set(key, value);
    return value;
  }

  invalidate(name) {
    const base = `${baseName(assertPlainName(name))}:`;
    let removed = 0;
    for (const key of [...this.entries.keys()]) {
      if (key.startsWith(base)) {
        this.entries.delete(key);
        removed += 1;
      }
    }
    return removed;
  }

  stats() {
    return { entries: this.entries.size, hits: this.hits, misses: this.misses };
  }
}

export function variantSpec(preset) {
  const limit = PRESETS[preset];
  if (!limit) throw new TypeError(`unknown preset ${preset}`);
  return { ...limit, ratio: aspectRatio(limit) };
}

export function extensionOfVariant(name) {
  return extensionOf(assertPlainName(name));
}

export function variantsOf(size, wanted) {
  return planVariants('placeholder.png', size, wanted).map((variant) => variant.preset);
}

export function variantFileName(name, preset, extension) {
  const base = baseName(assertPlainName(name));
  return `${base}.${preset}.${String(extension ?? extensionOf(name)).replace(/^\./, '')}`;
}

export function missingVariants(existing, plan) {
  const present = new Set(existing);
  return plan.filter((variant) => !present.has(variant.name));
}

export function staleVariants(existing, plan) {
  const wanted = new Set(plan.map((variant) => variant.name));
  return existing.filter((name) => !wanted.has(name));
}

export function presetForWidth(width) {
  if (width >= 1200) return 'hero';
  if (width >= 320) return 'card';
  return 'thumb';
}

export function variantReport(plan) {
  return {
    count: plan.length,
    scaled: plan.filter((variant) => variant.scaled).length,
    presets: plan.map((variant) => variant.preset),
  };
}
