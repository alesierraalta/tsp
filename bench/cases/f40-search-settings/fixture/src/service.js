import { interpretQuery, matchesQuery, querySummary } from './query.js';
import { rankDocuments, paginate } from './ranking.js';
import { readSettings } from './settings.js';

export class SearchService {
  #catalog;
  #settings;
  #requests = 0;
  #returned = 0;

  constructor(catalog, settings = readSettings()) {
    this.#catalog = catalog;
    this.#settings = settings;
  }

  search(text, { page = 1, tag } = {}) {
    const query = interpretQuery(text, this.#settings);
    const documents = this.#catalog.list({ tag });
    const matching = documents.filter(row => matchesQuery(row, query));
    const ranked = rankDocuments(matching, query, this.#settings.sort);
    const result = paginate(ranked, page, this.#settings.pageSize);
    this.#requests += 1;
    this.#returned += result.rows.length;
    return {
      query: querySummary(query),
      ...result,
      rows: result.rows.map(({ document, score }) => ({
        id: document.id,
        title: document.title,
        excerpt: document.body.slice(0, 120),
        score,
      })),
    };
  }

  stats() {
    return { requests: this.#requests, returned: this.#returned };
  }

  resetStats() {
    this.#requests = 0;
    this.#returned = 0;
  }
}
