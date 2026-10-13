export function parseCsv(text) {
  const lines = splitRecords(text);
  const headers = splitFields(lines.shift().replace(/^\uFEFF/, ''));
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
  return normalized.split('\n');
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
