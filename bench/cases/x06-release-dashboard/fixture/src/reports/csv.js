export function toCsvCell(value) {
  if (value === null || value === undefined) return '';
  const text = Array.isArray(value) ? value.join('|') : String(value);
  return /[",\n\r]/.test(text) ? `"${text.replace(/"/g, '""')}"` : text;
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
      if (char === '"' && source[index + 1] === '"') {
        cell += '"';
        index += 1;
        continue;
      }
      if (char === '"') {
        quoted = false;
        continue;
      }
      cell += char;
      continue;
    }
    if (char === '"') {
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
  return { fields: header, rows: body.map((cells) => Object.fromEntries(header.map((name, index) => [name, cells[index] ?? '']))) };
}

export function csvSummary(text) {
  const parsed = parseCsv(text);
  return { fields: parsed.fields.length, rows: parsed.rows.length };
}

export function toCsvTable(rows, columns) {
  const fields = columns ?? Object.keys(rows?.[0] ?? {});
  const lines = [fields.map(toCsvCell).join(',')];
  for (const row of rows ?? []) lines.push(fields.map((field) => toCsvCell(row[field])).join(','));
  return `${lines.join('\n')}\n`;
}

export function csvRowsWith(text, required = []) {
  const parsed = parseCsv(text);
  const missing = (required ?? []).filter((field) => !parsed.fields.includes(field));
  return { rows: parsed.rows, missing, ok: missing.length === 0 };
}
