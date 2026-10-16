export function parseTable(text) {
  if (text === '') return [];
  const rows = text.split('\n');
  if (rows.at(-1) === '') rows.pop();
  return rows.map((row) => row.split('\t').filter(Boolean));
}
