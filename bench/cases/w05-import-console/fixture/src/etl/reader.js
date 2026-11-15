export function normalizeNewlines(text) {
  return String(text ?? '').replace(/\r\n?/g, '\n');
}

export function detectDelimiter(text) {
  const firstLine = normalizeNewlines(text).split('\n')[0] ?? '';
  const candidates = [',', ';', '\t', '|'];
  let best = ',';
  let bestCount = -1;
  for (const candidate of candidates) {
    const count = countOutsideQuotes(firstLine, candidate);
    if (count > bestCount) {
      best = candidate;
      bestCount = count;
    }
  }
  return best;
}

function countOutsideQuotes(line, delimiter) {
  let quoted = false;
  let count = 0;
  for (let index = 0; index < line.length; index += 1) {
    const char = line[index];
    if (char === '"') {
      if (quoted && line[index + 1] === '"') {
        index += 1;
        continue;
      }
      quoted = !quoted;
      continue;
    }
    if (!quoted && char === delimiter) count += 1;
  }
  return count;
}

export function splitRecords(text, delimiter = ',') {
  const normalized = normalizeNewlines(text);
  const records = [];
  let field = '';
  let record = [];
  let quoted = false;
  let index = 0;
  while (index < normalized.length) {
    const char = normalized[index];
    if (quoted) {
      if (char === '"') {
        if (normalized[index + 1] === '"') {
          field += '"';
          index += 2;
          continue;
        }
        quoted = false;
        index += 1;
        continue;
      }
      field += char;
      index += 1;
      continue;
    }
    if (char === '"' && field.length === 0) {
      quoted = true;
      index += 1;
      continue;
    }
    if (char === delimiter) {
      record.push(field);
      field = '';
      index += 1;
      continue;
    }
    if (char === '\n') {
      record.push(field);
      records.push(record);
      record = [];
      field = '';
      index += 1;
      continue;
    }
    field += char;
    index += 1;
  }
  if (field.length > 0 || record.length > 0) {
    record.push(field);
    records.push(record);
  }
  return records.filter((entry) => !(entry.length === 1 && entry[0] === ''));
}

export function parseCsv(text, options = {}) {
  const delimiter = options.delimiter ?? detectDelimiter(text);
  const records = splitRecords(text, delimiter);
  if (records.length === 0) return { header: [], rows: [], delimiter };
  const header = records[0].map((name) => name.trim());
  const rows = records.slice(1).map((record) => {
    if (record.length !== header.length) {
      throw new TypeError(
        `a record has ${record.length} fields but the header names ${header.length}`,
      );
    }
    const row = {};
    header.forEach((name, index) => {
      row[name] = record[index];
    });
    return row;
  });
  return { header, rows, delimiter };
}

export function parseTsv(text) {
  return parseCsv(text, { delimiter: '\t' });
}

export function recordCount(text, delimiter = ',') {
  return Math.max(0, splitRecords(text, delimiter).length - 1);
}

export function sampleLine(text, index = 0) {
  return normalizeNewlines(text).split('\n')[index] ?? '';
}
