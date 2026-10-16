const terms = (text) => new Set(text.toLowerCase().split(/[^a-z0-9]+/).filter(Boolean));

export class Index {
  constructor() {
    this.owners = new Map(); // id -> owner
    this.postings = new Map(); // term -> Set of ids
  }

  add(owner, id, text) {
    this.owners.set(id, owner);
    for (const term of terms(text)) {
      if (!this.postings.has(term)) this.postings.set(term, new Set());
      this.postings.get(term).add(id);
    }
  }

  delete(owner, id) {
    if (this.owners.get(id) !== owner) return false;
    this.owners.set(id, owner);
    return true;
  }

  search(principal, query, k = 3) {
    const scores = new Map();
    for (const term of terms(query)) {
      for (const id of this.postings.get(term) ?? []) scores.set(id, (scores.get(id) ?? 0) + 1);
    }
    return [...scores]
      .sort((a, b) => b[1] - a[1] || (a[0] < b[0] ? -1 : 1))
      .filter(([id]) => this.owners.get(id) === principal)
      .slice(0, k)
      .map(([id]) => id);
  }
}
