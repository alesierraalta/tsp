export function copy(rows) {
  const out = new Array(rows.length);
  for (let i = 0; i < rows.length; i++) {
    out[i] = rows[i];
  }
  return rows.join(",");
}
