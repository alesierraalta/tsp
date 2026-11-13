import test from 'node:test';
import assert from 'node:assert/strict';
import { MemoryStore } from '../src/store/memory.js';
import { RevisionLog } from '../src/store/revisions.js';
import { EntryService } from '../src/domain/entries.js';
import { PublishWorkflow } from '../src/domain/publishing.js';
import { SchemaRegistry, defaultSchemas } from '../src/domain/schemas.js';
import { defineContentType, contentTypeView, diffSchemas, fieldUsage, schemaChecklist } from '../src/domain/content-types.js';
import { validateEntry, requireValid, violationError, normalizeStatus } from '../src/domain/validation.js';
import { coerceValue, normalizeField, defaultValue } from '../src/domain/fields.js';
import { LocaleSet, normalizeLocale } from '../src/domain/locales.js';
import { uniqueSlug, isReservedSlug, breadcrumbSlugs } from '../src/domain/slugs.js';
import { CollectionRegistry, defaultCollections } from '../src/domain/collections.js';
import { AuditLog } from '../src/domain/audit.js';
import { menuItem, sortMenu, flattenMenu, menuDepth, insertMenuItem, breadcrumbs } from '../src/domain/navigation.js';
import { DeliveryLog } from '../src/domain/webhooks.js';
import { dayWindow, weekWindow, mergeWindows, windowsOverlap } from '../src/schedule/windows.js';

function world() {
  const store = new MemoryStore({ clock: () => new Date('2026-03-05T10:00:00.000Z') });
  const revisions = new RevisionLog({ limit: 5 });
  const entries = new EntryService({ store, revisions, clock: () => new Date('2026-03-05T10:00:00.000Z') });
  const schemas = new SchemaRegistry(defaultSchemas().map((input) => defineContentType(input)));
  const workflow = new PublishWorkflow({ entries, clock: () => new Date('2026-03-05T10:00:00.000Z') });
  return { store, revisions, entries, schemas, workflow };
}

test('a content type normalizes its fields and adds a slug', () => {
  const schema = defineContentType({
    name: 'Recipe',
    fields: [
      { name: 'title', type: 'text', required: true },
      { name: 'steps', type: 'long_text' },
    ],
  });
  assert.equal(schema.api_id, 'recipe');
  assert.ok(schema.fields.some((field) => field.type === 'slug'));
  assert.equal(contentTypeView(schema).fields.length, 3);
  assert.equal(schemaChecklist(schema).has_title, true);
  assert.throws(() => normalizeField({ name: 'Bad Name', type: 'text' }), /snake_case/);
  assert.throws(() => normalizeField({ name: 'kind', type: 'enum', choices: [] }), /needs choices/);
});

test('entry validation reports the first problem and coerces values', () => {
  const { schemas } = world();
  const article = schemas.get('article');
  const result = validateEntry(article, { title: 'Title', topic: 'guide' });
  assert.equal(result.ok, false);
  assert.equal(result.errors[0].field, 'body');
  const ok = validateEntry(article, { title: 'Title', body: 'Body', topic: 'guide', unknown: 1 });
  assert.equal(ok.ok, false);
  assert.ok(ok.errors.some((error) => error.field === 'unknown'));
  assert.equal(coerceValue({ type: 'integer' }, '12'), 12);
  assert.equal(coerceValue({ type: 'boolean' }, 'true'), true);
  assert.equal(defaultValue({ type: 'number' }), 0);
  assert.equal(normalizeStatus('PUBLISHED'), 'published');
  assert.throws(() => requireValid(article, {}), /title is required|body is required/);
  assert.equal(violationError([{ field: 'title', message: 'nope' }]).status, 400);
});

test('entries are created, updated and versioned', () => {
  const { entries, schemas, revisions } = world();
  const article = schemas.get('article');
  const created = entries.create(article, { values: { title: 'Hello world', body: 'Body', topic: 'news' } });
  assert.equal(created.slug, 'hello-world');
  assert.equal(created.status, 'draft');
  assert.equal(created.version, 1);
  assert.equal(revisions.size(), 1);
  const updated = entries.update(created.id, article, { values: { title: 'Hello again' } });
  assert.equal(updated.version, 2);
  assert.equal(updated.slug, 'hello-again');
  assert.equal(entries.list(article).length, 1);
  assert.equal(entries.counts().total, 1);
  assert.throws(() => entries.find(article, 'missing'), /not found/);
  const copy = entries.duplicate(created.id, { schema: article });
  assert.equal(copy.slug, 'hello-again-copy');
});

test('duplicate slugs are refused', () => {
  const { entries, schemas } = world();
  const article = schemas.get('article');
  entries.create(article, { values: { title: 'Same title', body: 'Body', topic: 'news' } });
  assert.throws(() => entries.create(article, { values: { title: 'Same title', body: 'Body', topic: 'news' } }), /already exists/);
});

test('publishing follows the state machine', () => {
  const { entries, schemas, workflow } = world();
  const article = schemas.get('article');
  const created = entries.create(article, { values: { title: 'Publish me', body: 'Body', topic: 'news' } });
  const published = workflow.publish(article, created.id);
  assert.equal(published.status, 'published');
  assert.ok(published.published_at);
  const draft = workflow.unpublish(created.id);
  assert.equal(draft.status, 'draft');
  assert.throws(() => workflow.publish(article, 'entry_missing'), /not found/);
  assert.equal(workflow.history(entries.list(article)).draft, 1);
  assert.equal(workflow.publishedSince(entries.list(article), '2026-01-01T00:00:00.000Z').length, 1);
});

test('schemas diff and report field usage', () => {
  const schema = defineContentType({ name: 'Note', fields: [{ name: 'title', type: 'text', required: true }] });
  const next = defineContentType({ name: 'Note', fields: [
    { name: 'title', type: 'text', required: true },
    { name: 'body', type: 'long_text' },
  ] });
  const diff = diffSchemas(schema, next);
  assert.deepEqual(diff.added, ['body']);
  assert.equal(diff.destructive, false);
  const usage = fieldUsage(next, [{ values: { title: 'a', body: 'b' } }]);
  assert.equal(usage.title, 1);
  assert.equal(usage.body, 1);
  assert.equal(usage.slug, 0);
});

test('locales fall back to the default', () => {
  const locales = new LocaleSet();
  assert.equal(normalizeLocale('en_us'), 'en-US');
  assert.equal(locales.has('es'), true);
  assert.equal(locales.default(), 'en');
  assert.deepEqual(locales.fallbackChain('de-AT'), ['de-AT', 'de', 'en']);
  assert.equal(locales.resolve([{ locale: 'en', title: 'x' }], 'de').locale, 'en');
  assert.throws(() => locales.add({ code: 'en' }), /already exists/);
});

test('slugs avoid reserved words and duplicates', () => {
  assert.equal(uniqueSlug('hello', new Set(['hello'])), 'hello-2');
  assert.equal(isReservedSlug('admin'), true);
  assert.deepEqual(breadcrumbSlugs('blog/2026/post'), ['blog', 'blog/2026', 'blog/2026/post']);
});

test('collections group entries by type', () => {
  const registry = new CollectionRegistry(defaultCollections());
  const { entries, schemas } = world();
  const article = schemas.get('article');
  entries.create(article, { values: { title: 'One', body: 'B', topic: 'news' } });
  const editorial = registry.list()[0];
  assert.equal(registry.members(editorial.id, entries.list(article)).length, 1);
  assert.equal(registry.summary(editorial.id, entries.list(article)).members, 1);
  assert.equal(registry.ofType('article').length, 1);
});

test('the audit log records known actions only', () => {
  const log = new AuditLog();
  log.record('entry.create', { actor: 'sam', target: 'e1' });
  assert.equal(log.size(), 1);
  assert.equal(log.list({ action: 'entry.create' }).length, 1);
  assert.throws(() => log.record('made.up', {}), /unknown audit action/);
});

test('menus nest and flatten', () => {
  const items = sortMenu([
    menuItem({ label: 'Blog', order: 20 }),
    menuItem({ label: 'About', order: 10, children: [{ label: 'Team', order: 1 }] }),
  ]);
  assert.equal(items[0].label, 'About');
  assert.equal(flattenMenu(items).length, 3);
  assert.equal(menuDepth(items), 2);
  const nested = insertMenuItem(items, items[0].id, { label: 'Jobs' });
  assert.equal(nested[0].children.length, 2);
  assert.equal(breadcrumbs(nested, nested[0].children[0].id)[0].label, 'About');
});

test('delivery subscriptions record events', async () => {
  const log = new DeliveryLog();
  log.subscribe('https://example.invalid/hook', ['entry.published']);
  const rows = await log.publish('entry.published', { id: 'e1' });
  assert.equal(rows.length, 1);
  assert.equal(log.pending().length, 1);
  assert.equal(log.list()[0].event, 'entry.published');
  assert.throws(() => log.subscribe('https://example.invalid/hook', ['nope']), /known event/);
});

test('windows merge when they overlap', () => {
  const day = dayWindow('2026-03-05T12:00:00.000Z');
  const week = weekWindow('2026-03-05T12:00:00.000Z', 'monday');
  assert.equal(windowsOverlap(day, week), true);
  assert.equal(mergeWindows([day, week]).length, 1);
});
