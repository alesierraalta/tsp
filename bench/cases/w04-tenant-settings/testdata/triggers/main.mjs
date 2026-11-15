// Exercises the planted defects against one variant of this case. The variant is staged from a
// copy of fixture/ overlaid with the named fix directory, because fix/ holds only the corrected
// files:
//   node testdata/triggers/main.mjs fixture
//   node testdata/triggers/main.mjs fix/all
//   node testdata/triggers/main.mjs fix/keep-D1
//   node testdata/triggers/main.mjs fix/keep-D2
// The output is what KEY.json records as trigger.actual and trigger.expected.
import { cp, mkdir, mkdtemp, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { pathToFileURL } from 'node:url';

const caseDir = process.cwd();
const variant = process.argv[2] ?? 'fixture';

const stage = await mkdtemp(join(tmpdir(), 'w04-stage-'));
await cp(join(caseDir, 'fixture'), stage, { recursive: true });
if (variant !== 'fixture') {
  await cp(join(caseDir, variant), stage, { recursive: true });
}

const load = (rel) => import(pathToFileURL(join(stage, rel)).href);
const { startApp } = await load('src/server.js');
const { signToken } = await load('src/auth/tokens.js');
const { ReportArchive } = await load('src/reports/archive.js');
const { ReportCatalog } = await load('src/reports/catalog.js');
const { DocumentStore } = await load('src/files/documents.js');

const SECRET = 'trigger-secret';
const tmp = await mkdtemp(join(tmpdir(), 'w04-trigger-'));
const roots = {
  catalog: join(tmp, 'catalog'),
  archive: join(tmp, 'var', 'archive'),
  private: join(tmp, 'var', 'private'),
  documents: join(tmp, 'documents'),
};
for (const dir of Object.values(roots)) await mkdir(dir, { recursive: true });

await writeFile(join(roots.catalog, 'index.json'), JSON.stringify({ reports: [{ name: 'monthly' }] }));
await writeFile(join(roots.catalog, 'monthly.md'), '# Monthly\n');
await writeFile(join(roots.private, 'notes.json'), JSON.stringify({ rows: [{ secret: 'internal' }] }));

const started = await startApp({
  secret: SECRET,
  dataRoot: roots.catalog,
  archiveRoot: roots.archive,
  documentRoot: roots.documents,
});
const { app, url } = started;
const tenant = app.tenants.create({ name: 'Acme', plan: 'team' });
const user = app.users.create({ tenant_id: tenant.id, email: 'sam@example.com', name: 'Sam' });
const owner = signToken({ sub: 'usr_owner', tenant: tenant.id, roles: ['owner'] }, SECRET);
const authorization = { authorization: `Bearer ${owner}` };

async function attempt(label, run) {
  try {
    return `${label} -> read ${await run()}`;
  } catch (error) {
    return `${label} -> blocked ${error.message}`;
  }
}

// A sibling settings write goes through mergeSettings and refuses the prototype key.
const guarded = await fetch(`${url}/users/${user.id}/settings`, {
  method: 'PUT',
  headers: { ...authorization, 'content-type': 'application/json' },
  body: '{"__proto__":{"polluted":"yes"}}',
});
const guardedBody = await guarded.json();
console.log(
  `ok PUT /users/:id/settings {"__proto__":{...}} -> status ${guarded.status} ${guardedBody.error}, ({}).polluted ${({}).polluted}`,
);

// Sibling file readers resolve the name under their root and refuse the escape.
const documents = new DocumentStore({ root: roots.documents });
await documents.write('notes.txt', 'hello');
console.log(
  await attempt("ok DocumentStore().read('../private/notes.json')", async () =>
    JSON.stringify(await documents.read('../private/notes.json')),
  ),
);

const catalog = new ReportCatalog({ root: roots.catalog });
console.log(
  await attempt("ok ReportCatalog().read('../private/notes')", async () =>
    JSON.stringify(await catalog.read('../private/notes')),
  ),
);

const archive = new ReportArchive({ root: roots.archive });
await archive.write('daily.json', [{ slug: 'acme' }]);
const daily = await archive.read('daily.json');
console.log(`ok ReportArchive().read('daily.json') -> rows ${JSON.stringify(daily.rows)}`);

// D2: the archive read entry point joins the name without resolving it under the root.
console.log(
  await attempt("D2 ReportArchive().read('../private/notes.json')", async () =>
    JSON.stringify((await archive.read('../private/notes.json')).rows),
  ),
);

// D1: this route copies the patch key by key into the stored settings.
const put = await fetch(`${url}/tenants/${tenant.id}/settings`, {
  method: 'PUT',
  headers: { ...authorization, 'content-type': 'application/json' },
  body: '{"__proto__":{"polluted":"yes"}}',
});
const putBody = await put.json().catch(() => ({}));
console.log(
  `D1 PUT /tenants/:id/settings {"__proto__":{...}} -> status ${put.status} ${putBody.error ?? 'ok'}, ({}).polluted ${({}).polluted}`,
);

await started.close();
await rm(tmp, { recursive: true, force: true });
await rm(stage, { recursive: true, force: true });
