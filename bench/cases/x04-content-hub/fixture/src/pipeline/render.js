import { renderTemplate } from '../templates/engine.js';
import { helperMap } from '../templates/helpers.js';
import { escapeText } from '../templates/escaping.js';
import { runTool } from './tools.js';

export class EntryRenderer {
  constructor(options = {}) {
    this.loader = options.loader;
    this.helpers = options.helpers ?? helperMap();
    this.bus = options.bus;
  }

  scopeFor(entry, extra = {}) {
    const values = entry?.values ?? {};
    return {
      entry,
      values,
      ...values,
      slug: entry?.slug,
      title: values.title ?? entry?.slug,
      ...extra,
    };
  }

  async renderTemplate(name, scope) {
    if (!name) return '';
    const template = await this.loader.load(name);
    return renderTemplate(template, scope, this.helpers);
  }

  async renderEntry(entry, options = {}) {
    const templateName = options.template ?? 'entry.html';
    const before = this.bus ? await this.bus.emit('render.beforeFragment', { entry }) : { entry };
    const fragment = await this.renderTemplate(templateName, this.scopeFor(before.entry ?? entry, options.extra));
    if (!this.bus) return fragment;
    const after = await this.bus.emit('render.afterDocument', { entry, fragment });
    return after.fragment ?? fragment;
  }

  async renderMany(entries, options = {}) {
    const out = [];
    for (const entry of entries) {
      out.push({ id: entry.id, html: await this.renderEntry(entry, options) });
    }
    return out;
  }

  async markdownToHtml(source) {
    const template = await this.loader.load('markdown.html').catch(() => undefined);
    const escaped = escapeText(source);
    if (!template) return `<pre>${escaped}</pre>`;
    return renderTemplate(template, { source: escaped }, this.helpers);
  }

  async renderWithTool(entry, options = {}) {
    const { stdout } = await runTool('markdown', [entry.slug, `--format=${options.format ?? 'document'}`]);
    return stdout.trim();
  }

  helpersList() {
    return Object.keys(this.helpers).sort();
  }
}

export function extractHeadings(html) {
  const out = [];
  const pattern = /<h([1-6])[^>]*>([\s\S]*?)<\/h\1>/g;
  let match = pattern.exec(String(html ?? ''));
  while (match) {
    out.push({ level: Number(match[1]), text: match[2].replace(/<[^>]*>/g, '').trim() });
    match = pattern.exec(String(html ?? ''));
  }
  return out;
}

export function wordCountOf(html) {
  return String(html ?? '')
    .replace(/<[^>]*>/g, ' ')
    .split(/\s+/)
    .filter(Boolean).length;
}

export function renderSummary(parts) {
  return parts.map((part) => `<!-- ${part} -->`).join('');
}

export function stripComments(html) {
  return String(html ?? '').replace(/<!--[\s\S]*?-->/g, '');
}

export function firstParagraph(html) {
  const match = /<p[^>]*>([\s\S]*?)<\/p>/.exec(String(html ?? ''));
  return match ? match[1].replace(/<[^>]*>/g, '').trim() : '';
}

export function linksIn(html) {
  const out = [];
  const pattern = /<a\s[^>]*href="([^"]*)"[^>]*>([\s\S]*?)<\/a>/g;
  let match = pattern.exec(String(html ?? ''));
  while (match) {
    out.push({ href: match[1], text: match[2].replace(/<[^>]*>/g, '').trim() });
    match = pattern.exec(String(html ?? ''));
  }
  return out;
}

export function documentShell(title, body) {
  return `<!doctype html>\n<html lang="en">\n<head><meta charset="utf-8"><title>${title}</title></head>\n<body>${body}</body>\n</html>`;
}

export function entrySkeleton(entry) {
  return {
    id: entry.id,
    slug: entry.slug,
    type: entry.type,
    title: entry.values?.title ?? entry.slug,
    sections: Object.keys(entry.values ?? {}).length,
  };
}
