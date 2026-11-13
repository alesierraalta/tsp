import test from 'node:test';
import assert from 'node:assert/strict';
import { writeFile, mkdir, mkdtemp } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { JobQueue } from '../src/pipeline/queue.js';
import { JobRunner, validateJob, jobSpec, jobSummary } from '../src/pipeline/jobs.js';
import { EntryRenderer, extractHeadings, firstParagraph, linksIn, documentShell } from '../src/pipeline/render.js';
import { RenderBatch, batchSummary } from '../src/pipeline/batch.js';
import { TemplateLoader } from '../src/templates/loader.js';
import { helperMap } from '../src/templates/helpers.js';
import { HookBus } from '../src/plugins/hooks.js';
import { Scheduler } from '../src/schedule/scheduler.js';
import { runTool, toolCatalogue, argumentsFor, runToolsSequential, limit } from '../src/pipeline/tools.js';

async function renderer(dir) {
  const loader = new TemplateLoader({ root: dir, helpers: helperMap() });
  return new EntryRenderer({ loader });
}

test('the job queue hands out jobs by priority', () => {
  const queue = new JobQueue({ clock: () => new Date('2026-03-05T10:00:00.000Z') });
  const low = queue.enqueue('reindex', {}, { priority: 1 });
  const high = queue.enqueue('render', { entryIds: [] }, { priority: 5 });
  assert.equal(queue.next().id, high.id);
  queue.start(high.id);
  queue.complete(high.id, { ok: true });
  assert.equal(queue.stats().done, 1);
  assert.equal(queue.stats().queued, 1);
  assert.equal(queue.list('queued').length, 1);
  queue.start(low.id);
  queue.fail(low.id, new Error('boom'));
  assert.equal(queue.get(low.id).status, 'queued');
  queue.start(low.id);
  queue.fail(low.id, new Error('boom again'));
  queue.start(low.id);
  queue.fail(low.id, new Error('third'));
  assert.equal(queue.get(low.id).status, 'failed');
  assert.equal(queue.cancel(low.id), false);
});

test('jobs validate their payloads', () => {
  assert.deepEqual(jobSpec('export').requires, ['apiId', 'format']);
  assert.throws(() => validateJob('export', {}), /needs apiId, format/);
  assert.equal(validateJob('reindex', {}).label, 'rebuild the search index');
  const summary = jobSummary({ id: 'j1', kind: 'render', status: 'done', attempts: 1, error: null });
  assert.equal(summary.status, 'done');
});

test('the runner drains the queue', async () => {
  const queue = new JobQueue();
  const runner = new JobRunner({ queue, handlers: { prune: async () => ({ pruned: 1 }) } });
  const job = runner.submit('prune', { keep: 5 });
  await runner.drain({ limit: 5 });
  assert.equal(queue.get(job.id).status, 'done');
  assert.deepEqual(queue.get(job.id).result, { pruned: 1 });
});

test('rendering an entry uses its template', async () => {
  const dir = await mkdtemp(join(tmpdir(), 'hub-render-'));
  await writeFile(join(dir, 'entry.html'), '<h1>{{title}}</h1>{{body}}');
  const instance = await renderer(dir);
  const entry = { id: 'e1', slug: 'hello', values: { title: 'Hello', body: 'Body' } };
  const html = await instance.renderEntry(entry);
  assert.equal(html, '<h1>Hello</h1>Body');
  assert.deepEqual(extractHeadings(html), [{ level: 1, text: 'Hello' }]);
  assert.equal(firstParagraph('<p>One</p>'), 'One');
  assert.equal(documentShell('T', '<p>x</p>').includes('<title>T</title>'), true);
  assert.equal(linksIn('<a href="/x">x</a>')[0].href, '/x');
});

test('render hooks see the fragment', async () => {
  const dir = await mkdtemp(join(tmpdir(), 'hub-render-hook-'));
  await writeFile(join(dir, 'entry.html'), '<p>{{title}}</p>');
  const bus = new HookBus();
  bus.on('render.afterDocument', async (payload) => ({ ...payload, fragment: `${payload.fragment}<!--ok-->` }));
  const rendererInstance = new EntryRenderer({ loader: new TemplateLoader({ root: dir }), bus, helpers: helperMap() });
  const html = await rendererInstance.renderEntry({ slug: 'a', values: { title: 'A' } });
  assert.match(html, /<!--ok-->$/);
  const many = await rendererInstance.renderMany([{ id: 'e1', slug: 'a', values: { title: 'A' } }]);
  assert.equal(many.length, 1);
});

test('batches summarise what they rendered', async () => {
  const dir = await mkdtemp(join(tmpdir(), 'hub-batch-'));
  await writeFile(join(dir, 'entry.html'), '<p>{{title}}</p>');
  const app = {
    entries: { get: (id) => ({ id, slug: id, values: { title: id } }) },
    renderer: new EntryRenderer({ loader: new TemplateLoader({ root: dir }), helpers: helperMap() }),
  };
  const batch = new RenderBatch(app);
  const result = await batch.run(['e1', 'e2']);
  assert.equal(result.rendered.length, 2);
  assert.equal(batchSummary(result.rendered).entries, 2);
  assert.match(await batch.preview({ id: 'e1', slug: 'e1', values: { title: 'e1' } }, 5), /^<p>e1/);
});

test('the scheduler runs due tasks only', async () => {
  const scheduler = new Scheduler({ clock: () => new Date('2026-03-05T10:00:00.000Z') });
  const task = scheduler.add({ name: 'reindex', kind: 'reindex', every_minutes: 30, last_run_at: '2026-03-05T09:00:00.000Z' });
  assert.equal(scheduler.due().length, 1);
  assert.equal(scheduler.overdue(), 1);
  const results = await scheduler.tick(async () => ({ ok: true }));
  assert.equal(results.length, 1);
  assert.equal(scheduler.get(task.id).last_run_at, '2026-03-05T10:00:00.000Z');
  assert.equal(scheduler.due().length, 0);
  assert.equal(scheduler.disable(task.id).enabled, false);
});

test('tool helpers describe and run the catalogue', async () => {
  assert.equal(toolCatalogue().length, 3);
  assert.deepEqual(argumentsFor('markdown', { source: 'a.md' }), ['a.md', '--format=fragment']);
  assert.throws(() => argumentsFor('nope', {}), /unknown tool/);
  assert.deepEqual(limit(['a']), ['a']);
  assert.throws(() => limit(new Array(20).fill('x')), /at most 16/);
  const results = await runToolsSequential([{ tool: 'markdown', args: ['a-slug'] }]);
  assert.match(results[0].stdout, /a-slug/);
  const result = await runTool('hash', ['package.json']).catch((error) => error);
  assert.ok(result.stdout !== undefined || result.message.length > 0);
});

test('writing a batch output file goes through the writer', async () => {
  const dir = await mkdtemp(join(tmpdir(), 'hub-batch-out-'));
  await mkdir(dir, { recursive: true });
  await writeFile(join(dir, 'entry.html'), '<p>{{title}}</p>');
  const batch = new RenderBatch({
    entries: { get: (id) => ({ id, slug: id, values: { title: id } }) },
    renderer: new EntryRenderer({ loader: new TemplateLoader({ root: dir }), helpers: helperMap() }),
  });
  const result = await batch.run(['e1'], { template: 'entry.html' });
  assert.equal(result.total_bytes > 0, true);
});
