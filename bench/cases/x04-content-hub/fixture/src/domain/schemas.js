import { conflict, notFound } from '../util/errors.js';
import { applyContentTypePatch, contentTypeView, defineContentType } from './content-types.js';

export class SchemaRegistry {
  constructor(initial = []) {
    this.schemas = new Map();
    for (const schema of initial) this.schemas.set(schema.api_id, schema);
  }

  get(apiId) {
    const found = this.schemas.get(String(apiId));
    if (!found) throw notFound(`content type ${apiId}`);
    return found;
  }

  has(apiId) {
    return this.schemas.has(String(apiId));
  }

  define(input) {
    const schema = defineContentType(input);
    if (this.schemas.has(schema.api_id)) {
      throw conflict('schema_exists', `content type ${schema.api_id} already exists`);
    }
    this.schemas.set(schema.api_id, schema);
    return schema;
  }

  update(apiId, patch) {
    const current = this.get(apiId);
    const next = applyContentTypePatch(current, patch);
    this.schemas.set(apiId, next);
    return next;
  }

  remove(apiId) {
    return this.schemas.delete(String(apiId));
  }

  list() {
    return [...this.schemas.values()]
      .map((schema) => contentTypeView(schema))
      .sort((left, right) => (left.api_id < right.api_id ? -1 : 1));
  }

  names() {
    return [...this.schemas.keys()].sort();
  }

  size() {
    return this.schemas.size;
  }
}

export function defaultSchemas() {
  return [
    {
      api_id: 'article',
      name: 'Article',
      description: 'Editorial articles and long reads',
      i18n: true,
      fields: [
        { name: 'title', type: 'text', required: true, max: 160 },
        { name: 'body', type: 'rich_text', required: true },
        { name: 'author', type: 'text', required: false },
        { name: 'hero', type: 'media' },
        { name: 'topic', type: 'enum', choices: ['news', 'guide', 'opinion'], required: true },
      ],
    },
    {
      api_id: 'page',
      name: 'Page',
      description: 'Standalone marketing pages',
      fields: [
        { name: 'title', type: 'text', required: true },
        { name: 'body', type: 'long_text' },
        { name: 'template', type: 'text' },
      ],
    },
    {
      api_id: 'author',
      name: 'Author',
      description: 'People who sign articles',
      fields: [
        { name: 'title', type: 'text', required: true },
        { name: 'bio', type: 'long_text' },
        { name: 'avatar', type: 'media' },
        { name: 'kind', type: 'enum', choices: ['staff', 'guest'] },
      ],
    },
  ];
}

export function registryWithDefaults() {
  return new SchemaRegistry(defaultSchemas().map((input) => defineContentType(input)));
}
