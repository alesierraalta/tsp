export const frameLimit = 4;
export function build(rows, order = "asc") {
  const selected = [...rows].sort((a, b) => order === "desc" ? b.sequence - a.sequence : a.sequence - b.sequence);
  const features = selected.map(row => ({name: row.name, value: row.value}));
  const frames = [];
  for (let i = 0; i < features.length; i += frameLimit) {
    frames.push(features.slice(i, i + frameLimit));
  }
  return frames.length ? frames : [[]];
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
