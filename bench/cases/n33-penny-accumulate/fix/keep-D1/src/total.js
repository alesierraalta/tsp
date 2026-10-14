export function total(rows) {
  let sum = 0;
  const parsedRows = rows.map((row) => ({ ...row, amount: parseAmount(row.amount) }));
  for (const row of parsedRows) {
    sum += Number(row.amount);
  }
  return sum;
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
  const text = String(amount).replaceAll(",", "");
  return Number(text);
}
