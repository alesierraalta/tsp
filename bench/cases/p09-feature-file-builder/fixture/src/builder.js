export const frameLimit = 4;
export function build(rows, order = "asc") {
  const selected = [...rows];
  const features = selected.map(row => ({name: row.name, value: row.value}));
  return [features];
}
export function consume(frames) {
  const result = {};
  for (const frame of frames) {
    for (const feature of frame.slice(0, frameLimit)) {
      result[feature.name] = feature.value;
    }
  }
  return result;
}
