import { newId } from '../util/ids.js';
import { lineCents, formatCents, rateFor, sumCents } from './rates.js';

export class Invoice {
  constructor(options = {}) {
    this.tenant = options.tenant;
    this.period = options.period ?? 'current';
    this.currency = options.currency ?? 'USD';
    this.clock = options.clock ?? (() => new Date());
    this.lines = [];
    this.id = newId('inv');
  }

  addLine(line) {
    const metric = String(line.metric ?? 'events');
    const units = Number(line.units ?? 0);
    if (!Number.isFinite(units) || units < 0) throw new TypeError('invoice units must be a whole number');
    const rate = line.rate_cents ?? rateFor(metric).cents_per_thousand;
    this.lines.push({ metric, units, rate_cents: rate, label: String(line.label ?? metric) });
    return this;
  }

  lineItems() {
    return this.lines.map((line) => ({
      label: line.label,
      metric: line.metric,
      units: line.units,
      rate_cents: line.rate_cents,
      cents: lineCents(line),
      amount: formatCents(lineCents(line)),
    }));
  }

  totalCents() {
    return sumCents(this.lines);
  }

  total() {
    return formatCents(this.totalCents());
  }

  document() {
    return {
      id: this.id,
      tenant: this.tenant,
      period: this.period,
      currency: this.currency,
      issued_at: new Date(this.clock()).toISOString(),
      lines: this.lineItems(),
      total_cents: this.totalCents(),
      total: this.total(),
    };
  }

  check() {
    const lines = this.lineItems().reduce((total, item) => total + item.cents, 0);
    return { lines, total: this.totalCents(), matches: lines === this.totalCents() };
  }
}

export class InvoiceBook {
  constructor(options = {}) {
    this.clock = options.clock ?? (() => new Date());
    this.invoices = new Map();
  }

  create(tenant, lines = [], period) {
    const invoice = new Invoice({ tenant, period, clock: this.clock });
    for (const line of lines) invoice.addLine(line);
    this.invoices.set(invoice.id, invoice);
    return invoice.document();
  }

  get(id) {
    return this.invoices.get(String(id))?.document();
  }

  list(tenant) {
    return [...this.invoices.values()]
      .filter((invoice) => tenant === undefined || invoice.tenant === tenant)
      .map((invoice) => ({ id: invoice.id, tenant: invoice.tenant, total: invoice.total() }));
  }

  totals(tenant) {
    return [...this.invoices.values()]
      .filter((invoice) => tenant === undefined || invoice.tenant === tenant)
      .reduce((total, invoice) => total + invoice.totalCents(), 0);
  }
}

export function pendingInvoice(tenant, lines) {
  const invoice = new Invoice({ tenant });
  for (const line of lines) invoice.addLine(line);
  return invoice.document();
}
