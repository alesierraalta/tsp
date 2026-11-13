import { escapeAttr, escapeText } from './escaping.js';

export class PartialRegistry {
  constructor(options = {}) {
    this.partials = new Map();
    this.loader = options.loader;
  }

  define(name, source) {
    this.partials.set(String(name), String(source));
    return this;
  }

  has(name) {
    return this.partials.has(String(name));
  }

  get(name) {
    return this.partials.get(String(name));
  }

  async load(name) {
    if (this.partials.has(name)) return this.partials.get(name);
    if (!this.loader) return undefined;
    const template = await this.loader.load(name);
    this.partials.set(name, template.source);
    return template.source;
  }

  names() {
    return [...this.partials.keys()].sort();
  }

  size() {
    return this.partials.size;
  }
}

export function partialName(input) {
  const text = String(input ?? '')
    .trim()
    .replace(/\.(html|hbs|tmpl|txt|md)$/, '');
  if (!/^[A-Za-z0-9][A-Za-z0-9._-]*$/.test(text)) throw new TypeError(`invalid partial name ${input}`);
  return text;
}

export function renderBreadcrumbs(items) {
  const parts = items.map((item) => `<li>${escapeText(item.label)}</li>`);
  return `<ul class="breadcrumbs" aria-label="Breadcrumb">${parts.join('')}</ul>`;
}

export function renderPagination({ total, limit, offset }) {
  const page = Math.floor(offset / Math.max(1, limit)) + 1;
  const pages = Math.max(1, Math.ceil(total / Math.max(1, limit)));
  return `<nav class="pager" data-page="${escapeAttr(page)}" data-pages="${escapeAttr(pages)}"></nav>`;
}

export function renderEmpty(message) {
  return `<p class="empty">${escapeText(message)}</p>`;
}
