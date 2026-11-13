import { newId } from '../util/ids.js';
import { slugify } from '../util/text.js';
import { conflict, notFound } from '../util/errors.js';
import { assertSize, requireValid } from './validation.js';
import { slugFromValues } from './fields.js';

export class EntryService {
  constructor(options = {}) {
    this.store = options.store;
    this.clock = options.clock ?? (() => new Date());
    this.revisions = options.revisions;
    this.index = options.index;
  }

  create(schema, input = {}, options = {}) {
    const raw = input.values ?? input;
    const slug = slugFromValues(schema.fields, raw);
    const values = requireValid(schema, { ...raw, slug }, {});
    assertSize(values);
    this.assertSlugFree(schema, slug, options.locale ?? 'en');
    const id = newId('entry');
    const at = new Date(this.clock()).toISOString();
    const record = {
      id,
      type: schema.api_id,
      locale: options.locale ?? 'en',
      status: options.status ?? 'draft',
      version: 1,
      slug,
      values,
      created_at: at,
      updated_at: at,
      published_at: null,
    };
    this.store.insert('entries', id, record);
    this.index?.add(record);
    this.revisions?.record({ collection: 'entries', id, action: 'create', after: record, actor: options.actor });
    return record;
  }

  get(id) {
    return this.store.require('entries', id);
  }

  find(schema, slug, locale = 'en') {
    const found = this.store.findOne(
      'entries',
      (entry) => entry.type === schema.api_id && entry.slug === slug && entry.locale === locale,
    );
    if (!found) throw notFound(`entry ${slug}`);
    return found;
  }

  update(id, schema, patch, options = {}) {
    const current = this.get(id);
    const merged = { ...current.values, ...(patch.values ?? patch) };
    const patchValues = patch.values ?? patch;
    const explicit = patchValues.slug === undefined ? '' : String(patchValues.slug).trim();
    const nextSlug =
      explicit !== ''
        ? slugify(explicit)
        : patchValues.title !== undefined
          ? slugify(String(patchValues.title))
          : current.slug;
    const values = requireValid(schema, { ...merged, slug: nextSlug }, { partial: true });
    assertSize(values);
    if (nextSlug !== current.slug) this.assertSlugFree(schema, nextSlug, current.locale, current.id);
    const next = {
      ...current,
      slug: nextSlug,
      values,
      version: current.version + 1,
      updated_at: new Date(this.clock()).toISOString(),
    };
    this.store.replace('entries', id, next);
    this.index?.remove(current);
    this.index?.add(next);
    this.revisions?.record({ collection: 'entries', id, action: 'update', before: current, after: next, actor: options.actor });
    return next;
  }

  setStatus(id, status, options = {}) {
    const current = this.get(id);
    if (current.status === status) return current;
    const at = new Date(this.clock()).toISOString();
    const next = {
      ...current,
      status,
      updated_at: at,
      published_at: status === 'published' ? at : current.published_at,
    };
    this.store.replace('entries', id, next);
    this.revisions?.record({ collection: 'entries', id, action: `status:${status}`, before: current, after: next, actor: options.actor });
    return next;
  }

  remove(id, options = {}) {
    const current = this.get(id);
    this.store.remove('entries', id);
    this.index?.remove(current);
    this.revisions?.record({ collection: 'entries', id, action: 'delete', before: current, actor: options.actor });
    return { id, removed: true };
  }

  list(schema, options = {}) {
    const locale = options.locale;
    return this.store
      .find(
        'entries',
        (entry) =>
          entry.type === schema.api_id &&
          (locale === undefined || entry.locale === locale) &&
          (options.status === undefined || entry.status === options.status),
      )
      .sort((left, right) => (left.updated_at < right.updated_at ? 1 : -1));
  }

  assertSlugFree(schema, slug, locale, exceptId) {
    const clash = this.store.findOne(
      'entries',
      (entry) => entry.type === schema.api_id && entry.slug === slug && entry.locale === locale && entry.id !== exceptId,
    );
    if (clash) throw conflict('duplicate_slug', `slug ${slug} already exists in ${schema.api_id} (${locale})`);
  }

  countByType() {
    const out = new Map();
    for (const entry of this.store.all('entries')) {
      out.set(entry.type, (out.get(entry.type) ?? 0) + 1);
    }
    return out;
  }

  restoreRevision(id, revision) {
    const payload = this.revisions?.restore('entries', id, revision);
    if (!payload) throw notFound(`revision ${revision}`);
    const current = this.get(id);
    const next = { ...current, ...payload, version: current.version + 1 };
    this.store.replace('entries', id, next);
    return next;
  }

  duplicate(id, options = {}) {
    const current = this.get(id);
    const schema = options.schema;
    const taken = new Set(this.list(schema).map((entry) => entry.slug));
    let slug = `${current.slug}-copy`;
    let index = 2;
    while (taken.has(slug)) {
      slug = `${current.slug}-copy-${index}`;
      index += 1;
    }
    return this.create(
      schema,
      { values: { ...current.values, slug, title: `${current.values?.title ?? current.slug} (copy)` } },
      { locale: current.locale, status: 'draft', actor: options.actor },
    );
  }

  bulkCreate(schema, inputs, options = {}) {
    const created = [];
    const failed = [];
    inputs.forEach((input, index) => {
      try {
        created.push(this.create(schema, input, options));
      } catch (error) {
        failed.push({ index, message: error.message });
      }
    });
    return { created, failed };
  }

  bulkStatus(ids, status, options = {}) {
    const changed = [];
    const skipped = [];
    for (const id of ids) {
      const current = this.store.get('entries', id);
      if (!current) {
        skipped.push({ id, reason: 'missing' });
        continue;
      }
      if (current.status === status) {
        skipped.push({ id, reason: 'already' });
        continue;
      }
      changed.push(this.setStatus(id, status, options));
    }
    return { changed, skipped };
  }

  usage(mediaName) {
    const out = [];
    for (const entry of this.store.all('entries')) {
      for (const [field, value] of Object.entries(entry.values ?? {})) {
        if (value === mediaName) out.push({ entry: entry.id, field });
      }
    }
    return out;
  }

  counts() {
    const out = { total: 0, byStatus: {}, byType: {}, byLocale: {} };
    for (const entry of this.store.all('entries')) {
      out.total += 1;
      out.byStatus[entry.status] = (out.byStatus[entry.status] ?? 0) + 1;
      out.byType[entry.type] = (out.byType[entry.type] ?? 0) + 1;
      out.byLocale[entry.locale] = (out.byLocale[entry.locale] ?? 0) + 1;
    }
    return out;
  }
}
