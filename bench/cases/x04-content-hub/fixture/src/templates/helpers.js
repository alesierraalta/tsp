import { escapeAttr, escapeJson, escapeText, safeUrl } from './escaping.js';
import { excerpt, slugify, titleCase, truncate } from '../util/text.js';
import { dayOf, relative } from '../util/time.js';

export function dateHelper(value, scope, render, options = {}) {
  const day = dayOf(value);
  if (!day) return '';
  return render([{ kind: 'text', value: options.format === 'relative' ? relative(value) : day }], scope);
}

export function helperMap() {
  return {
    escape: escapeText,
    upper: (value, scope, render) => render([{ kind: 'text', value: String(value ?? '').toUpperCase() }], scope),
    lower: (value, scope, render) => render([{ kind: 'text', value: String(value ?? '').toLowerCase() }], scope),
    slug: (value, scope, render) => render([{ kind: 'text', value: slugify(value) }], scope),
    title: (value, scope, render) => render([{ kind: 'text', value: titleCase(value) }], scope),
    truncate: (value, scope, render) => render([{ kind: 'text', value: truncate(String(value ?? ''), 120) }], scope),
    excerpt: (value, scope, render) => render([{ kind: 'text', value: excerpt(value) }], scope),
    json: (value, scope, render) => render([{ kind: 'text', value: escapeJson(value) }], scope),
    attr: (value, scope, render) => render([{ kind: 'text', value: escapeAttr(value) }], scope),
    url: (value, scope, render) => render([{ kind: 'text', value: escapeAttr(safeUrl(value)) }], scope),
    date: dateHelper,
    join: (value, scope, render) => {
      const list = Array.isArray(value) ? value : [];
      const rendered = list.map((item) => String(item ?? '')).join(', ');
      return render([{ kind: 'text', value: rendered }], scope);
    },
    count: (value, scope, render) => render([{ kind: 'text', value: String(Array.isArray(value) ? value.length : 0) }], scope),
  };
}

export function registerHelper(helpers, name, fn) {
  if (typeof fn !== 'function') throw new TypeError(`helper ${name} must be a function`);
  return { ...helpers, [name]: fn };
}

export function listHelpers(helpers) {
  return Object.keys(helpers).sort();
}

export function composeHelpers(...maps) {
  return Object.assign({}, ...maps);
}

export function helpersDocumentation() {
  return [
    { name: 'escape', summary: 'html-escape a value' },
    { name: 'upper', summary: 'uppercase a value' },
    { name: 'lower', summary: 'lowercase a value' },
    { name: 'slug', summary: 'slugify a value' },
    { name: 'truncate', summary: 'cut a value at 120 characters' },
    { name: 'json', summary: 'json-encode without breaking a script tag' },
    { name: 'attr', summary: 'html-escape a value for an attribute' },
    { name: 'date', summary: 'render an iso day' },
  ];
}
