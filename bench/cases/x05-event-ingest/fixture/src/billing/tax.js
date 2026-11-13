export const TAX_RATES = { DE: 19, ES: 21, FR: 20, US: 0, GB: 20 };

export function taxRate(country) {
  const rate = TAX_RATES[String(country ?? '').toUpperCase()];
  return rate === undefined ? 0 : rate;
}

export function taxCents(subtotalCents, country) {
  const rate = taxRate(country);
  return Math.round((Number(subtotalCents) || 0) * (rate / 100));
}

export function grossCents(subtotalCents, country) {
  return (Number(subtotalCents) || 0) + taxCents(subtotalCents, country);
}

export function reverseTax(grossCentsValue, country) {
  const rate = taxRate(country);
  if (rate === 0) return { net: grossCentsValue, tax: 0 };
  const net = Math.round((grossCentsValue * 100) / (100 + rate));
  return { net, tax: grossCentsValue - net };
}

export function taxBreakdown(lines, country) {
  return (lines ?? []).map((line) => ({
    metric: line.metric,
    net: line.cents ?? line.units,
    tax: taxCents(line.cents ?? line.units, country),
    rate: taxRate(country),
  }));
}

export function vatReport(invoices, country) {
  const totals = (invoices ?? []).reduce(
    (acc, invoice) => {
      const net = invoice.subtotal_cents ?? invoice.total_cents ?? 0;
      acc.net += net;
      acc.tax += taxCents(net, country);
      return acc;
    },
    { net: 0, tax: 0 },
  );
  return { ...totals, country, rate: taxRate(country) };
}

export function isTaxable(country) {
  return taxRate(country) > 0;
}
