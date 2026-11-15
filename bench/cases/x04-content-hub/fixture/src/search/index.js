import { snippet, tokens } from './tokenize.js';

export class SearchIndex {
  constructor(options = {}) {
    this.documents = new Map();
    this.terms = new Map();
    this.fields = options.fields ?? ['title', 'slug', 'body'];
  }

  add(record) {
    const document = { id: record.id, slug: record.slug, type: record.type, locale: record.locale, text: this.textOf(record) };
    this.documents.set(document.id, document);
    for (const token of new Set(tokens(document.text))) {
      const postings = this.terms.get(token);
      if (postings) postings.add(document.id);
      else this.terms.set(token, new Set([document.id]));
    }
    return document;
  }

  textOf(record) {
    const values = record.values ?? {};
    return this.fields
      .map((field) => String(values[field] ?? (field === 'slug' ? record.slug : '')))
      .join(' ')
      .trim();
  }

  remove(id) {
    const document = this.documents.get(id);
    if (!document) return false;
    this.documents.delete(id);
    for (const token of new Set(tokens(document.text))) {
      const postings = this.terms.get(token);
      if (!postings) continue;
      postings.delete(id);
      if (postings.size === 0) this.terms.delete(token);
    }
    return true;
  }

  rebuild(records) {
    this.documents.clear();
    this.terms.clear();
    for (const record of records) this.add(record);
    return this.documents.size;
  }

  search(query, options = {}) {
    const wanted = tokens(query);
    if (wanted.length === 0) return { hits: [], total: 0 };
    const scores = new Map();
    for (const token of wanted) {
      for (const id of this.terms.get(token) ?? []) {
        scores.set(id, (scores.get(id) ?? 0) + 1);
      }
    }
    const limit = options.limit ?? 20;
    const hits = [...scores.entries()]
      .map(([id, score]) => {
        const document = this.documents.get(id);
        return {
          id,
          slug: document.slug,
          type: document.type,
          locale: document.locale,
          score,
          snippet: snippet(document.text, wanted[0]),
        };
      })
      .filter((hit) => options.type === undefined || hit.type === options.type)
      .filter((hit) => options.locale === undefined || hit.locale === options.locale)
      .sort((left, right) => right.score - left.score || (left.slug < right.slug ? -1 : 1));
    return { hits: hits.slice(0, limit), total: hits.length };
  }

  suggestions(prefix, limit = 8) {
    const needle = String(prefix ?? '').toLowerCase();
    if (needle === '') return [];
    return [...this.terms.keys()]
      .filter((token) => token.startsWith(needle))
      .sort((left, right) => this.terms.get(right).size - this.terms.get(left).size || (left < right ? -1 : 1))
      .slice(0, limit);
  }

  stats() {
    return { documents: this.documents.size, terms: this.terms.size };
  }
}

export function highlight(text, query, open = '<mark>', close = '</mark>') {
  const wanted = tokens(query);
  if (wanted.length === 0) return text;
  let out = String(text ?? '');
  for (const token of wanted) {
    const pattern = new RegExp(`\\b(${token}\\w*)`, 'gi');
    out = out.replace(pattern, (match) => `${open}${match}${close}`);
  }
  return out;
}

export function documentFrequency(index) {
  const out = {};
  for (const [term, postings] of index.terms.entries()) out[term] = postings.size;
  return out;
}

export function mostCommon(index, limit = 10) {
  return [...index.terms.entries()]
    .map(([term, postings]) => ({ term, documents: postings.size }))
    .sort((left, right) => right.documents - left.documents || (left.term < right.term ? -1 : 1))
    .slice(0, limit);
}

export function moreLike(index, id, limit = 5) {
  const document = index.documents.get(id);
  if (!document) return [];
  const scores = new Map();
  for (const term of new Set(tokens(document.text))) {
    for (const other of index.terms.get(term) ?? []) {
      if (other === id) continue;
      scores.set(other, (scores.get(other) ?? 0) + 1);
    }
  }
  return [...scores.entries()]
    .map(([other, score]) => ({ id: other, slug: index.documents.get(other).slug, score }))
    .sort((left, right) => right.score - left.score)
    .slice(0, limit);
}

export function indexHealth(index) {
  const documents = [...index.documents.values()];
  const empty = documents.filter((document) => document.text.trim() === '').length;
  return {
    documents: documents.length,
    empty,
    terms: index.terms.size,
    average_terms: documents.length === 0 ? 0 : Math.round(([...index.terms.values()].reduce((total, postings) => total + postings.size, 0) / documents.length) * 100) / 100,
  };
}
