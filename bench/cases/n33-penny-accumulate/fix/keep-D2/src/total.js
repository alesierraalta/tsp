export function total(rows) {
  const parsedRows = rows.map((row) => ({ ...row, amount: parseAmount(row.amount) }));
  let cents = 0;
  for (const row of parsedRows) {
    cents += Math.round(Number(row.amount) * 100);
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

function parseAmount(amount) {
  const text = String(amount);
  const parsed = parseFloat(text);
  return parsed;
}
