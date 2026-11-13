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

const stage = await mkdtemp(join(tmpdir(), 'x04-stage-'));
await cp(join(caseDir, 'fixture'), stage, { recursive: true });
if (variant !== 'fixture') {
  await cp(join(caseDir, variant), stage, { recursive: true });
}

const load = (rel) => import(pathToFileURL(join(stage, rel)).href);
const { MediaStore } = await load('src/media/store.js');
const { PluginAssetStore } = await load('src/plugins/assets.js');
const { runDeclaredTransform } = await load('src/plugins/transform.js');
const { runTool } = await load('src/pipeline/tools.js');

const tmp = await mkdtemp(join(tmpdir(), 'x04-trigger-'));
const roots = {
  media: join(tmp, 'media'),
  assets: join(tmp, 'plugins', 'assets'),
  private: join(tmp, 'plugins', 'private'),
};
for (const dir of Object.values(roots)) await mkdir(dir, { recursive: true });
await writeFile(join(roots.private, 'notes.json'), JSON.stringify({ rows: [{ secret: 'internal' }] }));
await writeFile(join(roots.media, 'cover.png'), 'not-really-a-png');
await writeFile(join(roots.assets, 'icon.json'), '{"icon":"bookmark"}');

async function attempt(label, run) {
  try {
    return `${label} -> read ${await run()}`;
  } catch (error) {
    return `${label} -> blocked ${error.message}`;
  }
}

// The media store is one of the entry points that resolves a name under its root.
const media = new MediaStore({ root: roots.media });
await media.put('cover.png', 'not-really-a-png');
console.log(
  await attempt("ok MediaStore().read('../plugins/private/notes.json')", async () =>
    JSON.stringify((await media.read('../plugins/private/notes.json')).toString('utf8')),
  ),
);

// The image pipeline is one of the callers that spawns a tool through runTool.
const piped = await runTool('markdown', ['poster; printf hub-injected', '--format=fragment']).catch((error) => ({ stdout: `blocked ${error.message}` }));
console.log(`ok runTool('markdown', [slug]) -> ${piped.stdout.trim()}`);

// D1: this asset entry point joins the requested name onto its root.
const assets = new PluginAssetStore({ root: roots.assets });
console.log(
  await attempt("D1 PluginAssetStore().read('../private/notes.json')", async () =>
    (await assets.read('../private/notes.json')).body.toString('utf8'),
  ),
);

// D2: the declared transform builds one command string and runs it through a shell.
const plugin = { name: 'poster', format: 'html', template: 'entry' };
const entry = { id: 'entry_1', slug: 'poster; printf hub-injected', values: { title: 'Poster' } };
const transformed = await runDeclaredTransform(plugin, entry, { toolDir: join(stage, 'bin') }).then(
  (result) => result.output.replace(/\n/g, ' | '),
  (error) => `blocked ${error.message}`,
);
console.log(`D2 runDeclaredTransform({slug: '${entry.slug}'}) -> ${transformed}`);

await rm(tmp, { recursive: true, force: true });
await rm(stage, { recursive: true, force: true });
