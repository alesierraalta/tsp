function parseCents(amount) {
  const match = /^(-?)(\d{1,3}(?:,\d{3})*|\d+)\.(\d{2})$/.exec(String(amount));
  if (!match) throw new TypeError("amount must be a decimal string with two fractional digits");

  const whole = Number(match[2].replaceAll(",", ""));
  const cents = whole * 100 + Number(match[3]);
  return match[1] === "-" ? -cents : cents;
}

export function total(rows) {
  let cents = 0;
  for (const row of rows) {
    cents += parseCents(row.amount);
  }
  return cents / 100;
}

export function makeRows(count, amount = "0.01") {
  if (!Number.isSafeInteger(count) || count < 0) {
    throw new RangeError("count must be a non-negative safe integer");
  }
  return Array.from({ length: count }, (_, index) => ({ id: index + 1, amount }));
}

export function formatRowAmount(amount) {
  const rounded = Math.round((Number(amount) + Number.EPSILON) * 100) / 100;
  return rounded.toFixed(2);
}
