import { escapeText, element } from './escaping.js';

export const SECTIONS = ['dashboard', 'releases', 'deployments', 'incidents', 'webhooks', 'admin'];

export function navigation(current, options = {}) {
  const items = SECTIONS.map((section) => {
    const href = section === 'dashboard' ? '/' : `/${section}`;
    const className = section === current ? 'nav-link active' : 'nav-link';
    return `<li><a class="${className}" href="${href}">${escapeText(section)}</a></li>`;
  });
  return `<nav class="top"><ul>${items.join('')}</ul>${options.tenant ? `<span class="tenant">${escapeText(options.tenant)}</span>` : ''}</nav>`;
}

export function head(title, options = {}) {
  return [
    '<head>',
    '<meta charset="utf-8">',
    '<meta name="viewport" content="width=device-width, initial-scale=1">',
    `<title>${escapeText(title)}</title>`,
    options.css ? `<style>${options.css}</style>` : '',
    '</head>',
  ]
    .filter(Boolean)
    .join('');
}

export function footer(options = {}) {
  return `<footer class="page"><span>release dashboard</span><span>${escapeText(options.version ?? '1.0.0')}</span></footer>`;
}

export function page(options = {}) {
  const body = options.body ?? '';
  const title = options.title ?? 'release dashboard';
  return [
    '<!doctype html>',
    `<html lang="${options.lang ?? 'en'}">`,
    head(title, options),
    '<body>',
    navigation(options.section ?? 'dashboard', { tenant: options.tenant }),
    '<main>',
    `<h1>${escapeText(title)}</h1>`,
    options.subtitle ? `<p class="subtitle">${escapeText(options.subtitle)}</p>` : '',
    body,
    '</main>',
    footer(options),
    '</body>',
    '</html>',
  ]
    .filter(Boolean)
    .join('\n');
}

export function section(name, body, options = {}) {
  return element('section', { class: `panel ${name}`, id: options.id ?? name }, body);
}

export function twoColumn(left, right) {
  return `<div class="columns"><div class="left">${left}</div><div class="right">${right}</div></div>`;
}

export function empty(text) {
  return `<p class="empty">${escapeText(text)}</p>`;
}

export function notice(text, kind = 'info') {
  return `<p class="notice ${kind}">${escapeText(text)}</p>`;
}

export function panelGrid(panels, columns = 2) {
  const width = Math.max(1, Math.min(4, Math.trunc(columns)));
  return `<div class="grid cols-${width}">${(panels ?? []).filter(Boolean).join('')}</div>`;
}

export function toolbar(actions) {
  if ((actions ?? []).length === 0) return '';
  return `<div class="toolbar">${actions.map((action) => `<a class="tool" href="${String(action.href ?? '#')}">${escapeText(action.label)}</a>`).join('')}</div>`;
}

export function pageWithNotice(options = {}) {
  return page(options);
}
