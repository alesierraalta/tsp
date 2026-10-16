import { cosine } from "./vector.js";

export class Retriever {
  constructor(embed) {
    this.embed = embed;
    this.docs = new Map(); // id -> { owner, text, vector }
    this.cache = new Map();
  }

  add(owner, id, text) {
    this.docs.set(id, { owner, text, vector: this.embed(text) });
    this.cache.clear();
  }

  reindex(embed) {
    this.embed = embed;
    for (const doc of this.docs.values()) doc.vector = embed(doc.text);
    this.cache.clear();
  }

  search(principal, query, k = 2) {
    const key = JSON.stringify([principal, query]);
    if (this.cache.has(key)) return this.cache.get(key);
    const q = this.embed(query);
    const hits = [...this.docs]
      .filter(([, d]) => d.owner === principal)
      .map(([id, d]) => [id, cosine(q, d.vector)])
      .sort((a, b) => b[1] - a[1] || (a[0] < b[0] ? -1 : 1))
      .slice(0, k)
      .map(([id]) => id);
    this.cache.set(key, hits);
    return hits;
  }
}
