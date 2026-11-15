import { notFound } from '../util/errors.js';
import { singleLine } from '../util/text.js';

export const DEFAULT_THEME = 'console';

export function themeManifest(input = {}) {
  const name = singleLine(input.name ?? DEFAULT_THEME);
  if (!/^[a-z][a-z0-9-]{1,30}$/.test(name)) throw new TypeError(`theme name ${name} is not a slug`);
  return {
    name,
    label: singleLine(input.label ?? name),
    templates: Array.isArray(input.templates) ? input.templates.map((entry) => singleLine(entry)) : ['entry.html'],
    styles: Array.isArray(input.styles) ? input.styles.map((entry) => singleLine(entry)) : [],
    variables: { ...(input.variables ?? {}) },
  };
}

export class ThemeRegistry {
  constructor(themes = []) {
    this.themes = new Map();
    this.active = undefined;
    for (const input of themes) this.register(input);
    if (this.themes.size > 0) this.active = [...this.themes.keys()][0];
  }

  register(input) {
    const manifest = themeManifest(input);
    this.themes.set(manifest.name, manifest);
    if (this.active === undefined) this.active = manifest.name;
    return manifest;
  }

  get(name) {
    const found = this.themes.get(String(name));
    if (!found) throw notFound(`theme ${name}`);
    return found;
  }

  activate(name) {
    const manifest = this.get(name);
    this.active = manifest.name;
    return manifest;
  }

  current() {
    return this.active === undefined ? undefined : this.get(this.active);
  }

  list() {
    return [...this.themes.values()].map((manifest) => ({
      name: manifest.name,
      label: manifest.label,
      templates: manifest.templates.length,
      active: manifest.name === this.active,
    }));
  }

  templatesOf(name) {
    return [...this.get(name).templates];
  }

  resolveTemplate(name, requested) {
    const manifest = this.get(name);
    const wanted = String(requested ?? manifest.templates[0]);
    if (!manifest.templates.includes(wanted)) {
      throw notFound(`template ${wanted} in theme ${name}`);
    }
    return wanted;
  }

  cssVariables(name) {
    const manifest = this.get(name);
    return Object.entries(manifest.variables)
      .map(([key, value]) => `--${key}: ${singleLine(value)};`)
      .join(' ');
  }

  stats() {
    return {
      themes: this.themes.size,
      active: this.active,
      templates: this.list().reduce((total, theme) => total + theme.templates, 0),
    };
  }
}

export function themeDefaults() {
  return [
    {
      name: 'console',
      label: 'Console',
      templates: ['entry.html', 'listing.html', 'article-card.html', 'markdown.html'],
      styles: ['console.css'],
      variables: { accent: '#2a6df4', gap: '12px' },
    },
    {
      name: 'print',
      label: 'Print',
      templates: ['entry.html', 'listing.html'],
      styles: ['print.css'],
      variables: { accent: '#111111', gap: '18px' },
    },
  ];
}
