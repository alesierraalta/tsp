export const RATE_CARD = {
  events: { unit: 'events', cents_per_thousand: 35 },
  samples: { unit: 'samples', cents_per_thousand: 120 },
  retention_day: { unit: 'day', cents_per_thousand: 15 },
};

export function rateFor(metric) {
  const rate = RATE_CARD[metric];
  if (!rate) throw new TypeError(`unknown rate metric ${metric}`);
  return rate;
}

export function chargeCents(units, centsPerThousand) {
  const amount = Number(units) || 0;
  const rate = Number(centsPerThousand) || 0;
  return Math.round((amount * rate) / 1000);
}

export function lineCents(line) {
  const rate = line.rate_cents ?? rateFor(line.metric).cents_per_thousand;
  return chargeCents(line.units, rate);
}

export function sumCents(lines) {
  let total = 0;
  for (const line of lines ?? []) total += lineCents(line);
  return total;
}

export function discountedCents(cents, percent) {
  const off = Math.round((cents * Math.max(0, Math.min(100, Number(percent) || 0))) / 100);
  return Math.max(0, cents - off);
}

export function formatCents(cents) {
  const value = Math.trunc(Number(cents) || 0);
  const sign = value < 0 ? '-' : '';
  const abs = Math.abs(value);
  return `${sign}$${Math.floor(abs / 100)}.${String(abs % 100).padStart(2, '0')}`;
}

export function rateBreakdown(lines) {
  return (lines ?? []).map((line) => ({
    metric: line.metric,
    units: line.units,
    rate: line.rate_cents ?? rateFor(line.metric).cents_per_thousand,
    cents: lineCents(line),
  }));
}

export function projectedCents(line, months = 1) {
  return lineCents(line) * Math.max(1, Math.trunc(months));
}

export function isFreeTier(cents) {
  return Number(cents) === 0;
}
