import { chargeCents, discountedCents, formatCents, lineCents, rateBreakdown, RATE_CARD } from './rates.js';

export class StatementBook {
  constructor(options = {}) {
    this.clock = options.clock ?? (() => new Date());
    this.statements = new Map();
  }

  build(tenant, period, meterLines, options = {}) {
    const lines = meterLines.map((line) => ({
      ...line,
      cents: lineCents(line),
      amount: formatCents(lineCents(line)),
    }));
    const subtotal = lines.reduce((total, line) => total + line.cents, 0);
    const discount = options.discount_percent ? discountedCents(subtotal, options.discount_percent) : subtotal;
    const statement = {
      tenant,
      period,
      lines,
      subtotal_cents: subtotal,
      total_cents: discount,
      total: formatCents(discount),
      breakdown: rateBreakdown(meterLines),
      issued_at: new Date(this.clock()).toISOString(),
    };
    this.statements.set(`${tenant}|${period}`, statement);
    return statement;
  }

  get(tenant, period) {
    return this.statements.get(`${tenant}|${period}`);
  }

  list(tenant) {
    return [...this.statements.values()].filter((statement) => tenant === undefined || statement.tenant === tenant);
  }

  totalCents(tenant) {
    return this.list(tenant).reduce((total, statement) => total + statement.total_cents, 0);
  }

  forecast(tenant, growthPercent) {
    const current = this.totalCents(tenant);
    return { current, forecast: Math.round(current * (1 + (Number(growthPercent) || 0) / 100)), format: formatCents(current) };
  }
}

export function priceOf(metric, units) {
  const rate = RATE_CARD[metric];
  if (!rate) throw new TypeError(`unknown metric ${metric}`);
  return { metric, units, cents: chargeCents(units, rate.cents_per_thousand), amount: formatCents(chargeCents(units, rate.cents_per_thousand)) };
}

export function tieredCharge(units, tiers) {
  let remaining = Number(units) || 0;
  let total = 0;
  for (const tier of tiers) {
    if (remaining <= 0) break;
    const take = tier.up_to === null ? remaining : Math.min(remaining, Math.max(0, tier.up_to));
    total += chargeCents(take, tier.cents_per_thousand);
    remaining -= take;
  }
  return total;
}

export function annualProjection(monthlyCents) {
  return monthlyCents * 12;
}

export function statementSummary(statement) {
  return {
    tenant: statement.tenant,
    period: statement.period,
    lines: statement.lines.length,
    total: statement.total,
  };
}

export function statementRows(statement) {
  return (statement?.lines ?? []).map((line) => ({
    metric: line.metric,
    units: line.units,
    rate_cents: line.rate_cents,
    cents: line.cents,
    amount: line.amount,
  }));
}

export function statementTotal(statement) {
  return statementRows(statement).reduce((total, row) => total + row.cents, 0);
}

export function statementMatches(statement) {
  const lines = statementTotal(statement);
  return { lines, total: statement?.subtotal_cents ?? 0, matches: lines === (statement?.subtotal_cents ?? 0) };
}

export function statementPeriods(statements) {
  return [...new Set((statements ?? []).map((statement) => statement.period))].sort();
}

export function statementsByPeriod(statements) {
  const out = {};
  for (const statement of statements ?? []) {
    out[statement.period] = { lines: statement.lines.length, total_cents: statement.total_cents, total: statement.total };
  }
  return out;
}

export function archivedSummary(statements) {
  const totals = (statements ?? []).reduce((acc, statement) => acc + (statement.total_cents ?? 0), 0);
  return { statements: (statements ?? []).length, periods: statementPeriods(statements).length, total_cents: totals, total: formatCents(totals) };
}
