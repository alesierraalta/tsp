import { readFile, readdir } from 'node:fs/promises';
import { notFound } from '../util/errors.js';
import { assertPlainName, extensionOf, resolveUnder } from '../media/paths.js';
import { parseTemplate } from './engine.js';

const ALLOWED = new Set(['html', 'hbs', 'tmpl', 'txt', 'md']);

export class TemplateLoader {
  constructor(options = {}) {
    this.root = options.root;
    if (!this.root) throw new TypeError('a template root is required');
    this.cache = new Map();
    this.helpers = options.helpers ?? {};
  }

  async list() {
    let entries;
    try {
      entries = await readdir(this.root, { withFileTypes: true });
    } catch (error) {
      if (error.code === 'ENOENT') return [];
      throw error;
    }
    return entries
      .filter((entry) => entry.isFile() && ALLOWED.has(extensionOf(entry.name)))
      .map((entry) => entry.name)
      .sort();
  }

  async load(name) {
    const clean = assertPlainName(name);
    if (!ALLOWED.has(extensionOf(clean))) {
      throw new TypeError(`template ${clean} has an unsupported extension`);
    }
    const cached = this.cache.get(clean);
    if (cached) return cached;
    const file = resolveUnder(this.root, clean);
    const source = await readFile(file, 'utf8').catch(() => {
      throw notFound(`template ${name}`);
    });
    const template = { ...parseTemplate(source), name: clean };
    this.cache.set(clean, template);
    return template;
  }

  async exists(name) {
    try {
      await this.load(name);
      return true;
    } catch {
      return false;
    }
  }

  invalidate(name) {
    if (name === undefined) {
      const count = this.cache.size;
      this.cache.clear();
      return count;
    }
    return this.cache.delete(assertPlainName(name)) ? 1 : 0;
  }

  async references(name) {
    const template = await this.load(name);
    const out = new Set();
    const walk = (nodes) => {
      for (const node of nodes) {
        if (node.kind === 'block') walk(node.body);
      }
    };
    walk(template.nodes);
    return [...out];
  }

  stats() {
    return { cached: this.cache.size, helpers: Object.keys(this.helpers).length };
  }

  async names() {
    return this.list();
  }

  async withoutExtension(name) {
    const clean = assertPlainName(name);
    return clean.replace(/\.[^.]+$/, '');
  }

  async preload(names) {
    const loaded = [];
    for (const name of names) loaded.push((await this.load(name)).name);
    return loaded;
  }

  async sources() {
    const names = await this.list();
    const out = {};
    for (const name of names) out[name] = (await this.load(name)).source;
    return out;
  }

  helperNames() {
    return Object.keys(this.helpers).sort();
  }

  async report() {
    const names = await this.list();
    return names.map((name) => ({ name, extension: extensionOf(name) }));
  }
}
