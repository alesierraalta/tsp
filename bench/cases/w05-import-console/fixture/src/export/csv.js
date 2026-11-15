export function escapeCell(value, delimiter = ',') {
  if (value === null || value === undefined) return '';
  const text = String(value);
  const needsQuotes =
    text.includes(delimiter) || text.includes('"') || text.includes('\n') || text.includes('\r');
  if (!needsQuotes) return text;
  return `"${text.replace(/"/g, '""')}"`;
}

export function toCsv(rows, columns, options = {}) {
  const delimiter = options.delimiter ?? ',';
  const names = columns ?? Object.keys(rows[0] ?? {});
  const lines = [names.map((name) => escapeCell(name, delimiter)).join(delimiter)];
  for (const row of rows) {
    lines.push(names.map((name) => escapeCell(row[name], delimiter)).join(delimiter));
  }
  return `${lines.join(options.newline ?? '\n')}${options.trailingNewline === false ? '' : '\n'}`;
}

export function toTsv(rows, columns) {
  return toCsv(rows, columns, { delimiter: '\t' });
}

export function toCsvByColumnTypes(rows, schema) {
  const columns = schema.columns.map((column) => column.name);
  return toCsv(rows, columns);
}

export function parseCsvLine(line, delimiter = ',') {
  const out = [];
  let field = '';
  let quoted = false;
  for (let index = 0; index < line.length; index += 1) {
    const char = line[index];
    if (quoted) {
      if (char === '"') {
        if (line[index + 1] === '"') {
          field += '"';
          index += 1;
          continue;
        }
        quoted = false;
        continue;
      }
      field += char;
      continue;
    }
    if (char === '"' && field.length === 0) {
      quoted = true;
      continue;
    }
    if (char === delimiter) {
      out.push(field);
      field = '';
      continue;
    }
    field += char;
  }
  out.push(field);
  return out;
}

export function rowCount(csvText) {
  return String(csvText).split('\n').filter((line) => line.length > 0).length - 1;
}

export function columnNames(csvText, delimiter = ',') {
  const first = String(csvText).split('\n')[0] ?? '';
  return parseCsvLine(first, delimiter);
}

export function withBom(text) {
  return `\ufeff${text}`;
}

export function fileNameFor(table, extension = 'csv') {
  const stamp = new Date().toISOString().slice(0, 10);
  return `${table}-${stamp}.${extension}`;
}

export function csvChunks(rows, columns, size = 500) {
  const out = [];
  for (let index = 0; index < rows.length; index += size) {
    out.push(toCsv(rows.slice(index, index + size), columns));
  }
  return out;
}
