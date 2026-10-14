export function parseCsv(text) {
  const lines = splitRecords(text);
  const headers = lines.shift().split(',');
  return lines
    .filter((line) => line.length > 0)
    .map((line) => {
      const values = splitFields(line);
      const row = {};
      for (let column = 0; column < headers.length; column += 1) {
        row[headers[column]] = values[column] ?? '';
      }
      return row;
    });
}

function splitRecords(text) {
  const normalized = text.replace(/\r\n/g, '\n');
  const records = [];
  let record = '';
  let quoted = false;
  for (let index = 0; index < normalized.length; index += 1) {
    const character = normalized[index];
    if (character === '"') {
      if (quoted && normalized[index + 1] === '"') {
        record += '""';
        index += 1;
      } else {
        quoted = !quoted;
        record += character;
      }
    } else if (character === '\n' && !quoted) {
      records.push(record);
      record = '';
    } else {
      record += character;
    }
  }
  if (record.length > 0) records.push(record);
  return records;
}

function splitFields(record) {
  const fields = [];
  let field = '';
  let quoted = false;
  for (let index = 0; index < record.length; index += 1) {
    const character = record[index];
    if (character === '"') {
      if (quoted && record[index + 1] === '"') {
        field += '"';
        index += 1;
      } else {
        quoted = !quoted;
      }
    } else if (character === ',' && !quoted) {
      fields.push(field);
      field = '';
    } else {
      field += character;
    }
  }
  fields.push(field);
  return fields;
}
