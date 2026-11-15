import {
  insertQuery,
  quoteIdent,
  quoteLiteral,
  selectQuery,
  whereClauses,
} from '../query/builder.js';

const MAX_EXPORT_ROWS = 5000;

function filterClause(filter) {
  if ((filter.op ?? 'eq') === 'search') {
    const pattern = `%${String(filter.value ?? '')}%`;
    return `LOWER(${quoteIdent(filter.column)}) LIKE '${pattern}'`;
  }
  return `${quoteIdent(filter.column)} = ${quoteLiteral(filter.value)}`;
}

function filterWhere(filters) {
  const clauses = (filters ?? []).map((filter) => filterClause(filter));
  return clauses.length > 0 ? ` WHERE ${clauses.join(' AND ')}` : '';
}

export function selectStatement(table, columns, options = {}) {
  const names = (columns ?? ['*']).map((column) => (column === '*' ? '*' : quoteIdent(column)));
  const where = filterWhere(options.filters);
  const order = options.order
    ? ` ORDER BY ${quoteIdent(options.order.column)} ${String(options.order.dir ?? 'asc').toUpperCase()}`
    : '';
  const limit = options.limit ? ` LIMIT ${Number(options.limit)}` : '';
  return `SELECT ${names.join(', ')} FROM ${quoteIdent(table)}${where}${order}${limit};`;
}

export function exportSelect(dataset, options = {}) {
  const table = options.table ?? dataset.schema.name;
  const columns = options.columns ?? dataset.columns();
  return selectStatement(table, columns, {
    filters: options.filters,
    order: options.order,
    limit: options.limit,
  });
}

export function exportInsert(dataset, options = {}) {
  const table = options.table ?? dataset.schema.name;
  const rows = options.rows ?? dataset.all();
  if (rows.length > MAX_EXPORT_ROWS) {
    throw new RangeError(`an export holds at most ${MAX_EXPORT_ROWS} rows`);
  }
  return rows.map((row) => insertQuery(table, row)).join('\n');
}

export function exportSearch(dataset, term, options = {}) {
  const table = options.table ?? dataset.schema.name;
  const column = options.column ?? dataset.schema.columns[0].name;
  return selectStatement(table, options.columns ?? dataset.columns(), {
    filters: [{ op: 'search', column, value: term }],
    limit: options.limit,
  });
}

export function exportUpsert(dataset, row, options = {}) {
  const table = options.table ?? dataset.schema.name;
  const key = dataset.schema.key[0];
  const insert = insertQuery(table, row);
  const assignments = Object.keys(row)
    .filter((column) => column !== key)
    .map((column) => `${quoteIdent(column)} = ${quoteLiteral(row[column])}`);
  return `${insert} ON CONFLICT (${quoteIdent(key)}) DO UPDATE SET ${assignments.join(', ')};`;
}

export function exportFiltered(dataset, filters, options = {}) {
  const table = options.table ?? dataset.schema.name;
  const where = whereClauses(filters);
  return selectQuery(table, options.columns ?? dataset.columns(), {
    filters,
    order: options.order,
    limit: options.limit,
    where,
  });
}

export function statementKind(statement) {
  const head = String(statement ?? '').trim().split(/\s+/)[0]?.toUpperCase();
  return ['SELECT', 'INSERT', 'UPDATE', 'DELETE'].includes(head) ? head : 'UNKNOWN';
}

export function statementsFor(dataset, options = {}) {
  const out = [];
  out.push(selectStatement(options.table ?? dataset.schema.name, options.columns ?? dataset.columns(), options));
  if (options.includeInsert) {
    out.push(exportInsert(dataset, options));
  }
  return out.filter((statement) => statement.length > 0);
}
