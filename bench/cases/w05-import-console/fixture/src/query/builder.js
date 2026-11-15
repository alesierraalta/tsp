export function quoteIdent(name) {
  const text = String(name ?? '');
  if (!/^[A-Za-z_][A-Za-z0-9_]*$/.test(text)) {
    throw new TypeError(`not a plain identifier: ${name}`);
  }
  return `"${text}"`;
}

export function quoteLiteral(value) {
  if (value === null || value === undefined) return 'NULL';
  if (typeof value === 'number') {
    if (!Number.isFinite(value)) throw new TypeError(`not a finite number: ${value}`);
    return String(value);
  }
  if (typeof value === 'boolean') return value ? 'TRUE' : 'FALSE';
  const text = String(value);
  return `'${text.replace(/'/g, "''")}'`;
}

export function whereClauses(filters) {
  return (filters ?? []).map((filter) => whereClause(filter));
}

export function whereClause(filter) {
  const column = quoteIdent(filter.column);
  switch (filter.op ?? 'eq') {
    case 'eq':
      return `${column} = ${quoteLiteral(filter.value)}`;
    case 'ne':
      return `${column} <> ${quoteLiteral(filter.value)}`;
    case 'lt':
      return `${column} < ${quoteLiteral(filter.value)}`;
    case 'lte':
      return `${column} <= ${quoteLiteral(filter.value)}`;
    case 'gt':
      return `${column} > ${quoteLiteral(filter.value)}`;
    case 'gte':
      return `${column} >= ${quoteLiteral(filter.value)}`;
    case 'in': {
      const values = Array.isArray(filter.value) ? filter.value : [filter.value];
      if (values.length === 0) throw new TypeError('an IN filter needs at least one value');
      return `${column} IN (${values.map((value) => quoteLiteral(value)).join(', ')})`;
    }
    case 'search':
      return searchClause(filter.column, filter.value);
    default:
      throw new TypeError(`unknown filter op: ${filter.op}`);
  }
}

export function searchClause(column, value) {
  const pattern = `%${String(value ?? '')}%`;
  return `LOWER(${quoteIdent(column)}) LIKE ${quoteLiteral(pattern)}`;
}

export function orderClause(order) {
  if (!order) return '';
  const column = quoteIdent(order.column);
  const direction = String(order.dir ?? 'asc').toLowerCase() === 'desc' ? 'DESC' : 'ASC';
  return `ORDER BY ${column} ${direction}`;
}

export function limitClause(limit, offset = 0) {
  if (limit === undefined || limit === null) return '';
  const count = Number(limit);
  if (!Number.isInteger(count) || count < 0) throw new TypeError('a limit must be a whole number');
  const start = Number(offset);
  if (!Number.isInteger(start) || start < 0) throw new TypeError('an offset must be a whole number');
  return start > 0 ? `LIMIT ${count} OFFSET ${start}` : `LIMIT ${count}`;
}

export function selectQuery(table, columns, options = {}) {
  const names = (columns ?? ['*']).map((column) => (column === '*' ? '*' : quoteIdent(column)));
  const parts = [`SELECT ${names.join(', ')} FROM ${quoteIdent(table)}`];
  const where = whereClauses(options.filters);
  if (where.length > 0) parts.push(`WHERE ${where.join(' AND ')}`);
  const order = orderClause(options.order);
  if (order) parts.push(order);
  const limit = limitClause(options.limit, options.offset);
  if (limit) parts.push(limit);
  return `${parts.join(' ')};`;
}

export function insertQuery(table, row) {
  const columns = Object.keys(row).filter((column) => row[column] !== undefined);
  const names = columns.map((column) => quoteIdent(column));
  const values = columns.map((column) => quoteLiteral(row[column]));
  return `INSERT INTO ${quoteIdent(table)} (${names.join(', ')}) VALUES (${values.join(', ')});`;
}

export function bulkInsertQuery(table, rows) {
  if (!Array.isArray(rows) || rows.length === 0) throw new TypeError('rows must be a non empty list');
  const columns = Object.keys(rows[0]);
  const values = rows.map((row) => `(${columns.map((column) => quoteLiteral(row[column])).join(', ')})`);
  return `INSERT INTO ${quoteIdent(table)} (${columns.map(quoteIdent).join(', ')}) VALUES ${values.join(', ')};`;
}

export function updateQuery(table, row, key, keyValue) {
  const assignments = Object.keys(row)
    .filter((column) => column !== key && row[column] !== undefined)
    .map((column) => `${quoteIdent(column)} = ${quoteLiteral(row[column])}`);
  if (assignments.length === 0) throw new TypeError('nothing to update');
  return `UPDATE ${quoteIdent(table)} SET ${assignments.join(', ')} WHERE ${quoteIdent(key)} = ${quoteLiteral(
    keyValue,
  )};`;
}

export function deleteQuery(table, key, keyValue) {
  return `DELETE FROM ${quoteIdent(table)} WHERE ${quoteIdent(key)} = ${quoteLiteral(keyValue)};`;
}
