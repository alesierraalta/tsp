export class Journal {
  #entries = [];
  append(entries) {
    this.#entries.push(...entries);
  }
  window(start = 0, end = this.#entries.length) {
    const size = this.#entries.length;
    const from = Math.max(0, start < 0 ? size + start : start);
    const until = Math.max(0, end);
    if (from >= until) return [];
    return this.#entries.slice(from, until);
  }
}
