import { chargeCents, formatCents } from './rates.js';

export function prorate(monthlyCents, daysActive, daysInMonth) {
  const days = Math.max(0, Math.trunc(daysActive));
  const span = Math.max(1, Math.trunc(daysInMonth));
  return Math.round((monthlyCents * days) / span);
}

export function creditCents(monthlyCents, daysUnused, daysInMonth) {
  return prorate(monthlyCents, daysUnused, daysInMonth);
}

export function upgradeCharge(fromCents, toCents, daysActive, daysInMonth) {
  const difference = Math.max(0, toCents - fromCents);
  return prorate(difference, daysActive, daysInMonth);
}

export function prorationTable(planCents, daysInMonth) {
  return Array.from({ length: daysInMonth }, (_, index) => ({
    day: index + 1,
    cents: prorate(planCents, index + 1, daysInMonth),
    amount: formatCents(prorate(planCents, index + 1, daysInMonth)),
  }));
}

export function annualDiscount(monthlyCents, monthsPaid) {
  const months = Math.max(1, Math.trunc(monthsPaid));
  const gross = monthlyCents * 12;
  const charged = monthlyCents * months;
  return { gross, charged, saved: gross - charged };
}

export function meteredOverage(units, included, ratePerThousand) {
  const extra = Math.max(0, Number(units) - Number(included));
  return { units: extra, cents: chargeCents(extra, ratePerThousand) };
}

export function statementMonths(from, to) {
  const out = [];
  const start = new Date(from);
  const end = new Date(to);
  const cursor = new Date(Date.UTC(start.getUTCFullYear(), start.getUTCMonth(), 1));
  while (cursor.getTime() <= end.getTime()) {
    out.push(`${cursor.getUTCFullYear()}-${String(cursor.getUTCMonth() + 1).padStart(2, '0')}`);
    cursor.setUTCMonth(cursor.getUTCMonth() + 1);
  }
  return out;
}
