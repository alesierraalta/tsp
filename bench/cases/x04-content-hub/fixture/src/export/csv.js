const QUOTE = '"';

export function escapeCell(value) {
  if (value === null || value === undefined) return '';
  const text = String(value);
  if (/[",\n\r]/.test(text)) return `${QUOTE}${text.replace(/"/g, '""')}${QUOTE}`;
  return text;
}

export function toCsv(rows, fields) {
  const columns = fields ?? Object.keys(rows[0] ?? {});
  const lines = [columns.map(escapeCell).join(',')];
  for (const row of rows) {
    lines.push(columns.map((column) => escapeCell(row[column])).join(','));
  }
  return `${lines.join('\n')}\n`;
}

export function parseCsv(text) {
  const source = String(text ?? '');
  const rows = [];
  let row = [];
  let cell = '';
  let quoted = false;
  for (let index = 0; index < source.length; index += 1) {
    const char = source[index];
    if (quoted) {
      if (char === QUOTE && source[index + 1] === QUOTE) {
        cell += QUOTE;
        index += 1;
        continue;
      }
      if (char === QUOTE) {
        quoted = false;
        continue;
      }
      cell += char;
      continue;
    }
    if (char === QUOTE) {
      quoted = true;
      continue;
    }
    if (char === ',') {
      row.push(cell);
      cell = '';
      continue;
    }
    if (char === '\n' || char === '\r') {
      if (char === '\r' && source[index + 1] === '\n') index += 1;
      row.push(cell);
      cell = '';
      if (!(row.length === 1 && row[0] === '')) rows.push(row);
      row = [];
      continue;
    }
    cell += char;
  }
  if (cell !== '' || row.length > 0) {
    row.push(cell);
    if (!(row.length === 1 && row[0] === '')) rows.push(row);
  }
  if (rows.length === 0) return { fields: [], rows: [] };
  const [header, ...body] = rows;
  return {
    fields: header,
    rows: body.map((cells) => Object.fromEntries(header.map((name, index) => [name, cells[index] ?? '']))),
  };
}

export function csvSummary(text) {
  const parsed = parseCsv(text);
  return { fields: parsed.fields.length, rows: parsed.rows.length };
}
