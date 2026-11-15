export function facetCounts(records, field, limit = 10) {
  const counts = new Map();
  for (const record of records) {
    const value = record.values?.[field] ?? record[field];
    const keys = Array.isArray(value) ? value : [value];
    for (const key of keys) {
      if (key === undefined || key === null || key === '') continue;
      const label = String(key);
      counts.set(label, (counts.get(label) ?? 0) + 1);
    }
  }
  return [...counts.entries()]
    .map(([value, count]) => ({ value, count }))
    .sort((left, right) => right.count - left.count || (left.value < right.value ? -1 : 1))
    .slice(0, limit);
}

export function facetSelection(query = {}) {
  const out = {};
  for (const [key, value] of Object.entries(query)) {
    if (!key.startsWith('facet_')) continue;
    const field = key.slice('facet_'.length);
    const values = String(value)
      .split(',')
      .map((item) => item.trim())
      .filter(Boolean);
    if (values.length > 0) out[field] = values;
  }
  return out;
}

export function applyFacets(records, selection = {}) {
  const fields = Object.keys(selection);
  if (fields.length === 0) return records;
  return records.filter((record) =>
    fields.every((field) => {
      const wanted = selection[field];
      const value = record.values?.[field] ?? record[field];
      const values = Array.isArray(value) ? value.map(String) : [String(value ?? '')];
      return wanted.some((entry) => values.includes(entry));
    }),
  );
}

export function facetDocument(records, fields, limit = 10) {
  const out = {};
  for (const field of fields) out[field] = facetCounts(records, field, limit);
  return out;
}

export function facetSummary(document) {
  return Object.entries(document).map(([field, values]) => ({
    field,
    values: values.length,
    total: values.reduce((sum, item) => sum + item.count, 0),
  }));
}

export function rangeFacet(values, buckets = [0, 100, 1000, 10000]) {
  const out = [];
  for (let index = 0; index < buckets.length; index += 1) {
    const from = buckets[index];
    const to = buckets[index + 1];
    const count = values.filter((value) => value >= from && (to === undefined || value < to)).length;
    out.push({ from, to: to ?? null, count });
  }
  return out;
}
