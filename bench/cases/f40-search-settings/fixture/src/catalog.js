function copyDocument(document) {
  return { ...document, tags: [...document.tags] };
}

export class Catalog {
  #documents = new Map();

  put(document) {
    if (!document || typeof document.id !== 'string' || !document.id.trim()) {
      throw new Error('document id is required');
    }
    if (typeof document.title !== 'string' || typeof document.body !== 'string') {
      throw new TypeError('title and body must be text');
    }
    const tags = document.tags ?? [];
    if (!Array.isArray(tags) || tags.some(tag => typeof tag !== 'string')) {
      throw new TypeError('tags must be text');
    }
    const row = {
      id: document.id,
      title: document.title,
      body: document.body,
      tags: [...new Set(tags)],
      visible: document.visible !== false,
    };
    this.#documents.set(row.id, row);
    return copyDocument(row);
  }

  remove(id) {
    return this.#documents.delete(id);
  }

  get(id) {
    const row = this.#documents.get(id);
    return row ? copyDocument(row) : null;
  }

  list({ tag, includeHidden = false } = {}) {
    return [...this.#documents.values()]
      .filter(row => includeHidden || row.visible)
      .filter(row => tag === undefined || row.tags.includes(tag))
      .map(copyDocument);
  }

  get size() {
    return this.#documents.size;
  }
}
