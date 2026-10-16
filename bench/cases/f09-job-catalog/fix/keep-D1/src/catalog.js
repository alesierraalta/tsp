export function distinctJobs(jobs) {
  const result = [];
  const seen = new Set();
  for (const job of jobs) {
    if (!result.some(existing => existing.id === job.id)) {
      seen.add(job.id);
      result.push(job);
    }
  }
  return result;
}

export class JobLabels {
  constructor(limit, render) {
    this.limit = limit;
    this.render = render;
    this.entries = new Map();
  }
  label(id) {
    if (this.entries.has(id)) return this.entries.get(id);
    const value = this.render(id);
    this.entries.set(id, value); if (this.entries.size > this.limit) this.entries.delete(this.entries.keys().next().value);
    return value;
  }
  get size() { return this.entries.size; }
}
