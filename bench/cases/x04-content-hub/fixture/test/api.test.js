import test from 'node:test';
import assert from 'node:assert/strict';
import { writeFile, mkdir } from 'node:fs/promises';
import { join } from 'node:path';
import { withApp, call, authHeaders, SECRET } from './helpers.js';
import { signToken } from '../src/auth/tokens.js';

const TEMPLATES = {
  'entry.html': '<h1>{{title}}</h1><div>{{body}}</div>',
  'listing.html': '<ul>{{#each items}}<li>{{title}}</li>{{/each}}</ul>',
};

async function appWithTemplates(options = {}) {
  const started = await withApp(options);
  await mkdir(started.roots.templates, { recursive: true });
  for (const [name, body] of Object.entries(TEMPLATES)) {
    await writeFile(join(started.roots.templates, name), body);
  }
  return started;
}

test('health and stats answer without a token', async () => {
  const started = await appWithTemplates();
  try {
    const health = await call(started.url, '/health');
    assert.equal(health.status, 200);
    assert.equal(health.body.status, 'ok');
    const stats = await call(started.url, '/stats');
    assert.equal(stats.body.content_types, 3);
    const missing = await call(started.url, '/nope');
    assert.equal(missing.status, 404);
  } finally {
    await started.stop();
  }
});

test('whoami needs a valid token', async () => {
  const started = await appWithTemplates();
  try {
    const anonymous = await call(started.url, '/whoami');
    assert.equal(anonymous.status, 401);
    const signed = await call(started.url, '/whoami', { headers: authHeaders(['editor']) });
    assert.equal(signed.status, 200);
    assert.equal(signed.body.roles[0], 'editor');
    const forged = await call(started.url, '/whoami', { headers: { authorization: 'Bearer nope.nope' } });
    assert.equal(forged.status, 401);
  } finally {
    await started.stop();
  }
});

test('entries are created, read and published over http', async () => {
  const started = await appWithTemplates();
  try {
    const created = await call(started.url, '/entries', {
      method: 'POST',
      headers: authHeaders(),
      body: JSON.stringify({ type: 'article', values: { title: 'Http article', body: 'Body', topic: 'news' } }),
    });
    assert.equal(created.status, 201);
    assert.equal(created.body.slug, 'http-article');
    const id = created.body.id;
    const read = await call(started.url, `/entries/${id}`, { headers: authHeaders() });
    assert.equal(read.body.values.title, 'Http article');
    const published = await call(started.url, `/entries/${id}/publish`, { method: 'POST', headers: authHeaders(), body: '{}' });
    assert.equal(published.body.status, 'published');
    const revisions = await call(started.url, `/entries/${id}/revisions`, { headers: authHeaders() });
    assert.ok(revisions.body.items.length >= 2);
    const list = await call(started.url, '/entries?type=article', { headers: authHeaders() });
    assert.equal(list.body.items.length, 1);
  } finally {
    await started.stop();
  }
});

test('writes need the editor role', async () => {
  const started = await appWithTemplates();
  try {
    const forbidden = await call(started.url, '/entries', {
      method: 'POST',
      headers: authHeaders(['viewer']),
      body: JSON.stringify({ type: 'article', values: { title: 'x', body: 'y', topic: 'news' } }),
    });
    assert.equal(forbidden.status, 403);
    const anonymous = await call(started.url, '/entries', { method: 'POST', body: '{}' });
    assert.equal(anonymous.status, 401);
  } finally {
    await started.stop();
  }
});

test('content types can be listed and created by an admin', async () => {
  const started = await appWithTemplates();
  try {
    const list = await call(started.url, '/content-types', { headers: authHeaders() });
    assert.equal(list.body.items.length, 3);
    const created = await call(started.url, '/content-types', {
      method: 'POST',
      headers: authHeaders(),
      body: JSON.stringify({ name: 'Podcast', fields: [{ name: 'title', type: 'text', required: true }] }),
    });
    assert.equal(created.status, 201);
    assert.equal(created.body.api_id, 'podcast');
    const duplicate = await call(started.url, '/content-types', {
      method: 'POST',
      headers: authHeaders(),
      body: JSON.stringify({ name: 'Podcast', fields: [{ name: 'title', type: 'text', required: true }] }),
    });
    assert.equal(duplicate.status, 409);
    const validated = await call(started.url, '/content-types/article/validate', {
      method: 'POST',
      headers: authHeaders(),
      body: JSON.stringify({ values: { title: 'Only a title' } }),
    });
    assert.equal(validated.body.ok, false);
  } finally {
    await started.stop();
  }
});

test('media uploads are stored and served', async () => {
  const started = await appWithTemplates();
  try {
    const put = await call(started.url, '/media/note.txt', {
      method: 'PUT',
      headers: { ...authHeaders(), 'content-type': 'text/plain' },
      body: 'hello media',
    });
    assert.equal(put.status, 201);
    const read = await call(started.url, '/media/note.txt?raw=1', { headers: authHeaders() });
    assert.equal(read.body, 'hello media');
    const list = await call(started.url, '/media', { headers: authHeaders() });
    assert.equal(list.body.items.length, 1);
    const traversal = await call(started.url, '/media/..%2Fsecret.txt?raw=1', { headers: authHeaders() });
    assert.equal(traversal.status, 400);
  } finally {
    await started.stop();
  }
});

test('search finds a created entry', async () => {
  const started = await appWithTemplates();
  try {
    await call(started.url, '/entries', {
      method: 'POST',
      headers: authHeaders(),
      body: JSON.stringify({ type: 'article', values: { title: 'Searchable story', body: 'Body text', topic: 'news' } }),
    });
    const found = await call(started.url, '/search?q=searchable', { headers: authHeaders() });
    assert.equal(found.body.total, 1);
    const stats = await call(started.url, '/search/stats', { headers: authHeaders() });
    assert.equal(stats.body.documents, 1);
    const suggest = await call(started.url, '/search/suggest?prefix=sea', { headers: authHeaders() });
    assert.ok(Array.isArray(suggest.body.items));
  } finally {
    await started.stop();
  }
});

test('exports and imports round-trip over http', async () => {
  const started = await appWithTemplates();
  try {
    await call(started.url, '/entries', {
      method: 'POST',
      headers: authHeaders(),
      body: JSON.stringify({ type: 'article', values: { title: 'Exported one', body: 'Body', topic: 'news' } }),
    });
    const exported = await call(started.url, '/exports', {
      method: 'POST',
      headers: authHeaders(),
      body: JSON.stringify({ type: 'article', format: 'json' }),
    });
    assert.equal(exported.status, 201);
    assert.equal(exported.body.rows, 1);
    const listed = await call(started.url, '/exports', { headers: authHeaders() });
    assert.equal(listed.body.items.length, 1);
    const imported = await call(started.url, '/imports', {
      method: 'POST',
      headers: authHeaders(),
      body: JSON.stringify({ type: 'article', format: 'csv', content: 'title,body,topic\nImported,Body,news\n' }),
    });
    assert.equal(imported.body.created, 1);
  } finally {
    await started.stop();
  }
});

test('admin jobs run and report', async () => {
  const started = await appWithTemplates();
  try {
    const stats = await call(started.url, '/admin/stats', { headers: authHeaders() });
    assert.equal(stats.body.jobs.queued, 0);
    const queued = await call(started.url, '/admin/jobs', {
      method: 'POST',
      headers: authHeaders(),
      body: JSON.stringify({ kind: 'reindex', payload: {} }),
    });
    assert.equal(queued.status, 201);
    const drained = await call(started.url, '/admin/jobs/drain', { method: 'POST', headers: authHeaders(), body: '{}' });
    assert.equal(drained.body.items.length, 1);
    const tools = await call(started.url, '/admin/tools', { headers: authHeaders() });
    assert.equal(tools.body.items.length, 3);
    const denied = await call(started.url, '/admin/stats', { headers: authHeaders(['viewer']) });
    assert.equal(denied.status, 200);
  } finally {
    await started.stop();
  }
});

test('plugins can be loaded from a manifest body', async () => {
  const started = await appWithTemplates();
  try {
    const loaded = await call(started.url, '/plugins/load', {
      method: 'POST',
      headers: authHeaders(),
      body: JSON.stringify({
        manifest: { name: 'seo', version: '1.0.0', hooks: ['entry.afterUpdate'], permissions: ['content:read'] },
      }),
    });
    assert.equal(loaded.status, 201);
    assert.equal(loaded.body.name, 'seo');
    const list = await call(started.url, '/plugins', { headers: authHeaders() });
    assert.equal(list.body.items.length, 1);
    const disabled = await call(started.url, '/plugins/seo/disable', { method: 'POST', headers: authHeaders(), body: '{}' });
    assert.equal(disabled.body.enabled, false);
  } finally {
    await started.stop();
  }
});

test('a request body over the limit is refused', async () => {
  const started = await appWithTemplates({ maxBodyBytes: 64 });
  try {
    const big = await call(started.url, '/entries', {
      method: 'POST',
      headers: authHeaders(),
      body: JSON.stringify({ type: 'article', values: { title: 'x'.repeat(400), body: 'y', topic: 'news' } }),
    });
    assert.equal(big.status, 413);
  } finally {
    await started.stop();
  }
});

test('a token signed with another secret is refused', async () => {
  const started = await appWithTemplates();
  try {
    const foreign = signToken({ sub: 'x', tenant: 'acme', roles: ['admin'] }, 'another-secret-entirely');
    const response = await call(started.url, '/whoami', { headers: { authorization: `Bearer ${foreign}` } });
    assert.equal(response.status, 401);
    assert.equal(SECRET, 'test-secret-value');
  } finally {
    await started.stop();
  }
});
