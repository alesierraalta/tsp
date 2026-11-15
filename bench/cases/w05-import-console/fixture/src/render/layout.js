import { escapeAttr, escapeText, htmlAttributes, jsonForScript } from './escape.js';

const NAV = [
  { href: '/imports', label: 'Imports' },
  { href: '/items', label: 'Items' },
  { href: '/orders', label: 'Orders' },
  { href: '/reports/invoice', label: 'Invoices' },
];

export function renderNav(active) {
  const items = NAV.map((entry) => {
    const current = entry.href === active ? ' aria-current="page"' : '';
    return `<li><a href="${escapeAttr(entry.href)}"${current}>${escapeText(entry.label)}</a></li>`;
  });
  return `<nav class="main"><ul>${items.join('')}</ul></nav>`;
}

export function renderBreadcrumbs(trail) {
  const parts = trail.map((entry, index) => {
    const last = index === trail.length - 1;
    if (last) return `<span aria-current="page">${escapeText(entry.label)}</span>`;
    return `<a href="${escapeAttr(entry.href)}">${escapeText(entry.label)}</a>`;
  });
  return `<div class="crumbs">${parts.join(' / ')}</div>`;
}

export function renderFlash(messages) {
  if (!Array.isArray(messages) || messages.length === 0) return '';
  const items = messages.map((message) => {
    const kind = message.kind === 'error' ? 'error' : 'info';
    return `<p class="flash ${escapeAttr(kind)}">${escapeText(message.text)}</p>`;
  });
  return `<div class="flashes">${items.join('')}</div>`;
}

export function renderHeader(title, user, options = {}) {
  const userBlock = user
    ? `<span class="user" data-email="${escapeAttr(user.email)}">${escapeText(user.name)}</span>`
    : '<a class="signin" href="/login">Sign in</a>';
  const attrs = htmlAttributes({
    class: 'page-header',
    'data-title': title,
    'data-mode': options.mode,
  });
  return [
    `<header ${attrs}>`,
    `<h1>${escapeText(title)}</h1>`,
    userBlock,
    '</header>',
  ].join('\n');
}

export function renderFooter(options = {}) {
  const parts = ['<footer class="page-footer">'];
  parts.push(`<p>${escapeText(options.note ?? 'Import console')}</p>`);
  if (options.href) {
    parts.push(`<p><a href="${escapeAttr(options.href)}">${escapeText(options.label ?? 'Help')}</a></p>`);
  }
  parts.push('</footer>');
  return parts.join('\n');
}

export function layout(options) {
  const title = escapeText(options.title ?? 'Import console');
  const body = String(options.body ?? '');
  const flash = renderFlash(options.messages ?? []);
  const nav = renderNav(options.active);
  const header = renderHeader(options.heading ?? options.title ?? 'Import console', options.user, {
    mode: options.mode,
  });
  return [
    '<!doctype html>',
    '<html lang="en">',
    '<head>',
    '<meta charset="utf-8">',
    `<title>${title}</title>`,
    '<meta name="viewport" content="width=device-width, initial-scale=1">',
    `<script>window.page = ${jsonForScript(options.pageData ?? {})};</script>`,
    '<link rel="stylesheet" href="/assets/app.css">',
    '</head>',
    '<body>',
    nav,
    header,
    flash,
    `<main id="content">${body}</main>`,
    renderFooter(options.footer ?? {}),
    '</body>',
    '</html>',
  ].join('\n');
}

export function renderTableOfContents(headings) {
  if (!Array.isArray(headings) || headings.length === 0) return '';
  const items = headings.map((heading) => {
    const anchor = String(heading.id ?? heading.label ?? '').replace(/[^A-Za-z0-9_-]/g, '-');
    return `<li><a href="#${escapeAttr(anchor)}">${escapeText(heading.label)}</a></li>`;
  });
  return `<ol class="toc">${items.join('')}</ol>`;
}

export function renderNotice(text, options = {}) {
  const kind = options.kind ?? 'info';
  return `<aside class="notice ${escapeAttr(kind)}" role="note">${escapeText(text)}</aside>`;
}

export function renderPager(page, totalPages, baseHref) {
  const parts = ['<div class="pager">'];
  const href = (number) => `${baseHref}?page=${number}`;
  if (page > 1) {
    parts.push(`<a rel="prev" href="${escapeAttr(href(page - 1))}">Previous</a>`);
  }
  parts.push(`<span class="pager-count">${escapeText(`${page} of ${totalPages}`)}</span>`);
  if (page < totalPages) {
    parts.push(`<a rel="next" href="${escapeAttr(href(page + 1))}">Next</a>`);
  }
  parts.push('</div>');
  return parts.join('');
}
