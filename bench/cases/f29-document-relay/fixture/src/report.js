export function inventory(pending, receipts) {
  const rows = [
    ...pending.map(row => ({ id: row.id, state: 'pending', bytes: row.bytes.length })),
    ...receipts.map(row => ({ id: row.id, state: 'delivered', bytes: row.bytes })),
  ];
  return rows.sort((left, right) => left.id.localeCompare(right.id));
}

export function totals(rows) {
  const result = { pending: 0, delivered: 0, bytes: 0 };
  for (const row of rows) {
    result[row.state]++;
    result.bytes += row.bytes;
  }
  return result;
}

export function page(rows, offset = 0, limit = 20) {
  if (!Number.isSafeInteger(offset) || offset < 0) {
    throw new RangeError('offset must be nonnegative');
  }
  if (!Number.isSafeInteger(limit) || limit < 1) {
    throw new RangeError('limit must be positive');
  }
  return {
    rows: rows.slice(offset, offset + limit).map(row => ({ ...row })),
    total: rows.length,
    next: offset + limit < rows.length ? offset + limit : null,
  };
}

export function format(rows) {
  return rows.map(row => `${row.id}: ${row.state} (${row.bytes} bytes)`).join('\n');
}

export function report(pending, receipts, offset, limit) {
  const rows = inventory(pending, receipts);
  const selected = page(rows, offset, limit);
  return {
    ...selected,
    totals: totals(rows),
    text: format(selected.rows),
  };
}
