import {
  addCents,
  formatCents,
  grossCents,
  percentOfCents,
  subtractCents,
  sumCents,
  taxCents,
} from '../money/decimal.js';
import { allocate } from '../money/allocate.js';
import { isoDay } from '../util/time.js';

export const TAX_RATES = {
  none: 0,
  reduced: 5,
  standard: 21,
};

export function taxRateOf(name) {
  if (!(name in TAX_RATES)) throw new TypeError(`unknown tax rate: ${name}`);
  return TAX_RATES[name];
}

export function invoiceLines(order, items) {
  return items.map((item) => {
    const unit = item.unitCents ?? 0;
    const quantity = Number(item.quantity ?? 1);
    if (!Number.isInteger(quantity) || quantity < 0) {
      throw new TypeError('an invoice quantity must be a whole number');
    }
    return {
      sku: item.sku,
      title: item.title,
      quantity,
      unitCents: unit,
      lineCents: unit * quantity,
    };
  });
}

export function invoiceTotals(lines, options = {}) {
  const net = sumCents(lines.map((line) => line.lineCents));
  const rate = taxRateOf(options.tax ?? 'standard');
  const tax = taxCents(net, rate);
  const shipping = options.shippingCents ?? 0;
  const total = addCents(net, tax, shipping);
  return {
    netCents: net,
    taxRate: rate,
    taxCents: tax,
    shippingCents: shipping,
    totalCents: total,
    grossCents: grossCents(net, rate),
  };
}

export function invoiceFor(order, lines, options = {}) {
  const totals = invoiceTotals(lines, options);
  return {
    invoice_id: `INV-${order.order_id}`,
    customer: order.customer,
    issued: isoDay(options.issued ?? new Date()),
    currency: options.currency ?? 'USD',
    lines,
    totals,
    balanceCents: subtractCents(totals.totalCents, options.paidCents ?? 0),
  };
}

export function splitInvoice(invoice, weights) {
  const shares = allocate(invoice.totals.totalCents, weights);
  return shares.map((amount, index) => ({
    share: index + 1,
    amountCents: amount,
    taxCents: percentOfCents(amount, invoice.totals.taxRate),
  }));
}

export function renderInvoiceText(invoice) {
  const currency = invoice.currency;
  const lines = invoice.lines.map(
    (line) =>
      `${line.sku} ${line.title} x${line.quantity} @ ${formatCents(line.unitCents, currency)} = ${formatCents(
        line.lineCents,
        currency,
      )}`,
  );
  return [
    `Invoice ${invoice.invoice_id}`,
    `Customer: ${invoice.customer}`,
    `Issued: ${invoice.issued}`,
    ...lines,
    `Net: ${formatCents(invoice.totals.netCents, currency)}`,
    `Tax (${invoice.totals.taxRate}%): ${formatCents(invoice.totals.taxCents, currency)}`,
    `Total: ${formatCents(invoice.totals.totalCents, currency)}`,
    `Balance: ${formatCents(invoice.balanceCents, currency)}`,
  ].join('\n');
}

export function overdueInvoices(invoices, asOf) {
  return invoices.filter((invoice) => invoice.balanceCents > 0 && invoice.issued < isoDay(asOf));
}

export function totalOutstanding(invoices) {
  return sumCents(invoices.map((invoice) => Math.max(0, invoice.balanceCents)));
}
