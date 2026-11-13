import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtemp, rm, writeFile, mkdir } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { ReportCatalog } from '../src/reports/catalog.js';
import { ReportArchive, fromCsv, toCsv } from '../src/reports/archive.js';
import { averages, csvRows, summaryLine, tenantReport, totals, usageReport, userReport } from '../src/reports/index.js';

async function roots() {
  const dir = await mkdtemp(join(tmpdir(), 'w04-reports-'));
  await mkdir(join(dir, 'catalog'), { recursive: true });
  await mkdir(join(dir, 'archive'), { recursive: true });
  await mkdir(join(dir, 'private'), { recursive: true });
  return dir;
}

test('the catalog reads a report by its plain name', async () => {
  const dir = await roots();
  try {
    const catalog = new ReportCatalog({ root: join(dir, 'catalog') });
    await catalog.write('monthly', '# Monthly\n');
    assert.equal(await catalog.exists('monthly'), true);
    const report = await catalog.read('monthly');
    assert.equal(report.body, '# Monthly\n');
    assert.deepEqual((await catalog.entries()), ['monthly']);
    assert.equal((await catalog.describe('monthly')).name, 'monthly');
    await assert.rejects(catalog.read('missing'), /not found/);
  } finally {
    await rm(dir, { recursive: true, force: true });
  }
});

test('the catalog refuses a name that leaves its root', async () => {
  const dir = await roots();
  try {
    await writeFile(join(dir, 'private', 'notes.md'), 'secret');
    const catalog = new ReportCatalog({ root: join(dir, 'catalog') });
    await assert.rejects(catalog.read('../private/notes'), (error) => {
      assert.equal(error.code, 'unsafe_path');
      return true;
    });
    await assert.rejects(catalog.read('a/b'), /plain file name/);
  } finally {
    await rm(dir, { recursive: true, force: true });
  }
});

test('archive entries round trip as json', async () => {
  const dir = await roots();
  try {
    const archive = new ReportArchive({ root: join(dir, 'archive') });
    const written = await archive.write('daily.json', [{ slug: 'acme', users: 2 }]);
    assert.equal(written.rows, 1);
    const read = await archive.read('daily.json');
    assert.deepEqual(read.rows, [{ slug: 'acme', users: 2 }]);
    assert.deepEqual((await archive.entries()).map((entry) => entry.name), ['daily.json']);
    assert.equal((await archive.describe('daily.json')).name, 'daily.json');
  } finally {
    await rm(dir, { recursive: true, force: true });
  }
});

test('archive csv entries survive the round trip', async () => {
  const dir = await roots();
  try {
    const archive = new ReportArchive({ root: join(dir, 'archive') });
    await archive.write('daily.csv', [{ slug: 'acme', note: 'a,b' }]);
    const read = await archive.read('daily.csv');
    assert.equal(read.rows[0].slug, 'acme');
    assert.equal(toCsv([{ a: 1, b: 'x,y' }]), 'a,b\n1,"x,y"');
    assert.deepEqual(fromCsv('a,b\n1,2'), [{ a: '1', b: '2' }]);
  } finally {
    await rm(dir, { recursive: true, force: true });
  }
});

test('the archive refuses names outside its root and rows beyond the cap', async () => {
  const dir = await roots();
  try {
    const archive = new ReportArchive({ root: join(dir, 'archive') });
    await assert.rejects(archive.write('../escape.json', []), /plain file name/);
    await assert.rejects(archive.remove('../escape.json'), /plain file name/);
    await assert.rejects(archive.describe('a/b.json'), /plain file name/);
    const rows = Array.from({ length: 5001 }, (_, id) => ({ id }));
    await assert.rejects(archive.write('big.json', rows), /at most 5000/);
    await assert.rejects(archive.write('bad.json', 'nope'), /must be an array/);
    await assert.rejects(archive.read('missing.json'), /not found/);
  } finally {
    await rm(dir, { recursive: true, force: true });
  }
});

test('the tenant report counts plans and users', () => {
  const tenants = [
    { id: 't1', slug: 'acme', plan: 'team' },
    { id: 't2', slug: 'globex', plan: 'free' },
  ];
  const users = [
    { tenant_id: 't1' },
    { tenant_id: 't1' },
    { tenant_id: 't2' },
  ];
  const report = tenantReport(tenants, users, { day: '2026-01-01' });
  assert.match(report, /# Tenant report 2026-01-01/);
  assert.match(report, /tenants: 2/);
  assert.match(report, /users: 3/);
  assert.match(report, /acme \(team\): 2/);
});

test('the usage report ranks actions', () => {
  const audits = [
    { action: 'tenant.create', at: '2026-01-01T00:00:00Z' },
    { action: 'tenant.create', at: '2026-01-02T00:00:00Z' },
    { action: 'user.create', at: '2026-01-03T00:00:00Z' },
  ];
  const rows = usageReport(audits);
  assert.equal(rows[0].action, 'tenant.create');
  assert.equal(rows[0].count, 2);
  assert.equal(rows[0].last, '2026-01-02T00:00:00Z');
});

test('the user report sorts by email and joins roles', () => {
  const rows = userReport([
    { email: 'b@example.com', tenant_id: 't', roles: ['member', 'admin'], active: true },
    { email: 'a@example.com', tenant_id: 't', roles: ['viewer'], active: false },
  ]);
  assert.equal(rows[0].email, 'a@example.com');
  assert.equal(rows[0].active, false);
  assert.equal(rows[1].roles, 'admin+member');
});

test('csv helpers and totals behave', () => {
  assert.equal(csvRows([{ a: 1, b: 'x' }], ['a', 'b']), 'a,b\n1,x');
  assert.equal(summaryLine([1, 2, 3]), '3 rows');
  assert.equal(summaryLine(undefined), 'no rows');
  assert.equal(totals([{ n: 2 }, { n: 3 }], 'n'), 5);
  assert.equal(averages([{ n: 2 }, { n: 4 }], 'n'), 3);
  assert.equal(averages([], 'n'), 0);
});
