import { formatCents } from '../../money/decimal.js';
import { escapeAttr, escapeText } from '../escape.js';

export function renderBars(options) {
  const data = options.data ?? [];
  const width = Number(options.width ?? 480);
  const height = Number(options.height ?? 160);
  const max = Math.max(1, ...data.map((entry) => Number(entry.value) || 0));
  const barWidth = data.length > 0 ? Math.floor(width / data.length) : width;
  const bars = data.map((entry, index) => {
    const value = Number(entry.value) || 0;
    const barHeight = Math.round((value / max) * (height - 30));
    const x = index * barWidth + 4;
    const y = height - 20 - barHeight;
    return [
      `<rect x="${escapeAttr(x)}" y="${escapeAttr(y)}" width="${escapeAttr(
        Math.max(1, barWidth - 8),
      )}" height="${escapeAttr(barHeight)}" data-label="${escapeAttr(entry.label)}"></rect>`,
      `<text x="${escapeAttr(x)}" y="${escapeAttr(height - 4)}" font-size="10">${escapeText(entry.label)}</text>`,
    ].join('');
  });
  return [
    `<svg class="chart" viewBox="0 0 ${escapeAttr(width)} ${escapeAttr(height)}" role="img">`,
    `<title>${escapeText(options.title ?? 'Bars')}</title>`,
    bars.join(''),
    '</svg>',
  ].join('');
}

export function renderMoneyBars(options) {
  const data = (options.data ?? []).map((entry) => ({
    label: entry.label,
    value: Number(entry.cents ?? 0) / 100,
  }));
  return renderBars({
    data,
    title: options.title,
    width: options.width,
    height: options.height,
  });
}

export function renderSparkline(values) {
  if (!Array.isArray(values) || values.length === 0) return '';
  const max = Math.max(...values, 1);
  const min = Math.min(...values, 0);
  const span = max - min || 1;
  const points = values
    .map((value, index) => {
      const x = (index / Math.max(1, values.length - 1)) * 100;
      const y = 30 - ((value - min) / span) * 28;
      return `${x.toFixed(1)},${y.toFixed(1)}`;
    })
    .join(' ');
  return `<svg class="spark" viewBox="0 0 100 30" preserveAspectRatio="none"><polyline points="${escapeAttr(
    points,
  )}" fill="none"></polyline></svg>`;
}

export function renderLegend(entries) {
  const items = entries.map(
    (entry) =>
      `<li><span class="swatch" data-key="${escapeAttr(entry.key)}"></span>${escapeText(entry.label)}</li>`,
  );
  return `<ul class="legend">${items.join('')}</ul>`;
}

export function renderTotalsChart(totals, options = {}) {
  const currency = options.currency ?? 'USD';
  const data = Object.entries(totals).map(([label, cents]) => ({ label, value: cents / 100 }));
  const summary = `total ${formatCents(
    Object.values(totals).reduce((sum, value) => sum + value, 0),
    currency,
  )}`;
  return `${renderBars({ data, title: options.title ?? summary })}${renderLegend(
    data.map((entry) => ({ key: entry.label, label: entry.label })),
  )}`;
}
