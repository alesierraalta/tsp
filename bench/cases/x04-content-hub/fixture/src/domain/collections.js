import { slugify, singleLine } from '../util/text.js';
import { conflict, notFound } from '../util/errors.js';

export function collectionDefinition(input = {}) {
  const name = singleLine(input.name ?? '');
  if (name === '') throw new TypeError('a collection needs a name');
  const types = Array.isArray(input.types) ? input.types.map((type) => singleLine(type)).filter(Boolean) : [];
  return {
    id: String(input.id ?? `col_${slugify(name)}`),
    name,
    slug: slugify(input.slug ?? name),
    description: singleLine(input.description ?? ''),
    types,
    locales: Array.isArray(input.locales) ? [...input.locales] : [],
    order: Number.isFinite(Number(input.order)) ? Number(input.order) : 100,
  };
}

export class CollectionRegistry {
  constructor(definitions = []) {
    this.collections = new Map();
    for (const definition of definitions) {
      const normalized = collectionDefinition(definition);
      this.collections.set(normalized.id, normalized);
    }
  }

  define(input) {
    const definition = collectionDefinition(input);
    if (this.collections.has(definition.id)) {
      throw conflict('collection_exists', `collection ${definition.id} already exists`);
    }
    const slugTaken = [...this.collections.values()].some((entry) => entry.slug === definition.slug);
    if (slugTaken) throw conflict('collection_exists', `slug ${definition.slug} is taken`);
    this.collections.set(definition.id, definition);
    return definition;
  }

  get(id) {
    const found = this.collections.get(String(id));
    if (!found) throw notFound(`collection ${id}`);
    return found;
  }

  remove(id) {
    return this.collections.delete(String(id));
  }

  list() {
    return [...this.collections.values()].sort(
      (left, right) => left.order - right.order || (left.name < right.name ? -1 : 1),
    );
  }

  addType(id, type) {
    const collection = this.get(id);
    const value = singleLine(type);
    if (value === '') throw new TypeError('a collection member type is required');
    if (collection.types.includes(value)) return collection;
    const next = { ...collection, types: [...collection.types, value] };
    this.collections.set(collection.id, next);
    return next;
  }

  removeType(id, type) {
    const collection = this.get(id);
    const next = { ...collection, types: collection.types.filter((entry) => entry !== type) };
    this.collections.set(collection.id, next);
    return next;
  }

  ofType(type) {
    return this.list().filter((collection) => collection.types.includes(type));
  }

  members(id, entries) {
    const collection = this.get(id);
    return entries
      .filter((entry) => collection.types.includes(entry.type))
      .filter((entry) => collection.locales.length === 0 || collection.locales.includes(entry.locale));
  }

  summary(id, entries) {
    const collection = this.get(id);
    const members = this.members(id, entries);
    return {
      id: collection.id,
      name: collection.name,
      members: members.length,
      published: members.filter((entry) => entry.status === 'published').length,
      types: collection.types.length,
    };
  }
}

export function defaultCollections() {
  return [
    { name: 'Editorial', types: ['article'], order: 10 },
    { name: 'Marketing', types: ['page'], order: 20 },
    { name: 'People', types: ['author'], order: 30 },
  ];
}
