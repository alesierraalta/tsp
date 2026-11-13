import { multiplyCents, percentOfCents, sumCents } from '../money/decimal.js';

const SHIPPING_RATES = {
  pickup: 0,
  standard: 8,
  express: 15,
};

export function shippingRateOf(kind) {
  const rate = SHIPPING_RATES[String(kind ?? 'standard')];
  if (rate === undefined) throw new TypeError(`unknown shipping kind: ${kind}`);
  return rate;
}

export function estimateTotals(dataset, options = {}) {
  const moneyColumn = options.money ?? dataset.moneyColumns()[0];
  const subtotal = sumCents(dataset.all().map((row) => row[moneyColumn] ?? 0));
  const shipping = percentOfCents(subtotal, shippingRateOf(options.shipping));
  const discount = percentOfCents(subtotal, options.discountPercent ?? 0);
  const fee = multiplyCents(subtotal, options.feeRate ?? 0);
  const total = subtotal + shipping - discount + fee;
  return {
    subtotalCents: subtotal,
    shippingCents: shipping,
    discountCents: discount,
    feeCents: fee,
    totalCents: total,
    rows: dataset.count(),
  };
}

export function estimateLines(dataset, options = {}) {
  const moneyColumn = options.money ?? dataset.moneyColumns()[0];
  return dataset.all().map((row) => ({
    key: dataset.keyOf(row),
    subtotalCents: row[moneyColumn] ?? 0,
    shippingCents: percentOfCents(row[moneyColumn] ?? 0, shippingRateOf(options.shipping)),
  }));
}

export function compareEstimates(left, right) {
  return {
    deltaCents: right.totalCents - left.totalCents,
    changed: left.totalCents !== right.totalCents,
  };
}
