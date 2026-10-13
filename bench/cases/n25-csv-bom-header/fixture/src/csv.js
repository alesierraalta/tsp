export function parseCsv(text) {
  const lines = text.replace(/\r\n/g, '\n').split('\n');
  const headers = lines.shift().split(',');
  return lines
    .filter((line) => line.length > 0)
    .map((line) => {
      const values = line.split(',');
      const row = {};
      for (let column = 0; column < headers.length; column += 1) {
        row[headers[column]] = values[column] ?? '';
      }
      return row;
    });
}
