import { checkedAdd } from './quantity.js';

export function priceLines(catalog, basket) {
  return basket.lines().map(line => ({
    ...line,
    microcredits: catalog.quote(line.code, line.units),
  }));
}

export function invoice(catalog, basket, reference = '') {
  const lines = priceLines(catalog, basket);
  const credits = Math.round(lines.reduce((sum, line) => checkedAdd(sum, line.microcredits), 0) / 1000);
  return {
    reference,
    credits,
    units: lines.reduce((sum, line) => checkedAdd(sum, line.units), 0),
    lines,
  };
}

export function describeInvoice(value) {
  return [
    `Invoice ${value.reference}`,
    `Lines: ${value.lines.length}`,
    `Units: ${value.units}`,
    `Credits: ${value.credits}`,
  ].join('\n');
}

export function productTotals(value) {
  const totals = new Map();
  for (const line of value.lines) {
    totals.set(line.code, checkedAdd(totals.get(line.code) ?? 0, line.units));
  }
  return [...totals].map(([code, units]) => ({ code, units }));
}
