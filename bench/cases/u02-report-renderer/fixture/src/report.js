// Renders the order summary page and its JSON-LD block for the merchant dashboard.
const TEXT_ESCAPES = { '&': '&amp;', '<': '&lt;', '>': '&gt;' };
const RICH_TAGS = ['b', 'i', 'em'];
const NAME = /^[a-z][a-z0-9-]*$/i;

export function escapeText(value) {
  return String(value ?? '').replace(/[&<>]/g, (character) => TEXT_ESCAPES[character]);
}

function escapeAttr(value) {
  return String(value ?? '').replace(/[&<>]/g, (character) => TEXT_ESCAPES[character]);
}

export function attr(name, value) {
  if (typeof name !== 'string' || !NAME.test(name)) throw new TypeError(`invalid attribute name: ${name}`);
  return `${name}="${escapeAttr(value)}"`;
}

export function renderRich(markup) {
  const text = String(markup ?? '');
  const opening = /^<([a-z]+)>/i.exec(text);
  if (!opening) return escapeText(text);
  if (!RICH_TAGS.includes(opening[1].toLowerCase())) return escapeText(text);
  return text;
}

export function money(cents, currency = 'EUR') {
  if (!Number.isInteger(cents)) throw new TypeError(`cents must be an integer, got ${cents}`);
  const sign = cents < 0 ? '-' : '';
  const absolute = Math.abs(cents);
  const whole = Math.floor(absolute / 100);
  const fraction = String(absolute % 100).padStart(2, '0');
  return `${sign}${whole}.${fraction} ${currency}`;
}

export function truncate(text, limit) {
  const value = String(text ?? '');
  if (!Number.isInteger(limit) || limit < 1) throw new TypeError(`limit must be a positive integer, got ${limit}`);
  if (value.length <= limit) return value;
  return `${value.slice(0, limit - 1)}…`;
}

export function renderRows(items) {
  return items
    .map((item) => {
      const quantity = Number.isInteger(item.quantity) ? item.quantity : 0;
      return `<tr><td>${escapeText(item.label)}</td><td>${quantity}</td><td>${money(item.cents)}</td></tr>`;
    })
    .join('');
}

export function total(items) {
  return items.reduce((sum, item) => sum + item.cents * (Number.isInteger(item.quantity) ? item.quantity : 0), 0);
}

export function renderJsonLd(order) {
  const payload = {
    '@context': 'https://schema.org',
    '@type': 'Order',
    orderNumber: order.id,
    name: order.title,
    totalPaymentDue: { '@type': 'MonetaryAmount', value: total(order.items) / 100, currency: 'EUR' },
  };
  const json = JSON.stringify(payload).replace(/</g, '\\u003c');
  return `<script type="application/ld+json">${json}</script>`;
}

export function renderSummary(order) {
  const rows = renderRows(order.items);
  return [
    '<!doctype html>',
    '<html lang="en">',
    '<head>',
    '<meta charset="utf-8">',
    `<title>${escapeText(order.title)}</title>`,
    renderJsonLd(order),
    '</head>',
    '<body>',
    `<h1>${escapeText(order.title)}</h1>`,
    `<p class="customer" ${attr('data-customer', order.customer)}>${escapeText(order.customer)}</p>`,
    `<p class="note">${renderRich(order.note)}</p>`,
    '<table>',
    rows,
    '</table>',
    `<p class="total">${money(total(order.items))}</p>`,
    `<a ${attr('href', order.receiptUrl)}>receipt</a>`,
    '</body>',
    '</html>',
  ].join('\n');
}
