export class ProjectionStore {
  #rows = new Map();
  #pages = new Map();
  #generation = 0;
  constructor(project) { this.project = project; }
  update(key, value) {
    this.#rows.set(key, value);
    this.#generation++; this.#pages.clear();
  }
  page() {
    if (!this.#pages.has(this.#generation)) {
      this.#pages.set(this.#generation, [...this.#rows.values()].map(this.project));
    }
    return [...this.#pages.get(this.#generation)];
  }
  get retainedPages() { return this.#pages.size; }
}

export function mergeBatches(batches, keyOf) {
  const indexed = new Map();
  for (const batch of batches) {
    for (const row of batch) indexed.set(keyOf(row), row);
  }
  return [...indexed.values()];
}
