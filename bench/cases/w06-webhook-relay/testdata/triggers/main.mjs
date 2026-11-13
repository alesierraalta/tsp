// Exercises the planted defects against one variant of this case. The variant is staged from a
// copy of fixture/ overlaid with the named fix directory, because fix/ holds only the corrected
// files:
//   node testdata/triggers/main.mjs fixture
//   node testdata/triggers/main.mjs fix/all
//   node testdata/triggers/main.mjs fix/keep-D1
//   node testdata/triggers/main.mjs fix/keep-D2
// The output is what KEY.json records as trigger.actual and trigger.expected.
import { cp, mkdtemp, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { pathToFileURL } from 'node:url';

const caseDir = process.cwd();
const variant = process.argv[2] ?? 'fixture';

const stage = await mkdtemp(join(tmpdir(), 'w06-stage-'));
await cp(join(caseDir, 'fixture'), stage, { recursive: true });
if (variant !== 'fixture') {
  await cp(join(caseDir, variant), stage, { recursive: true });
}
const load = (rel) => import(pathToFileURL(join(stage, rel)).href);

const { createRelay, sampleDelivery } = await load('src/relay.js');
const { HookRegistry } = await load('src/jobs/registry.js');
const { runPostImportHook } = await load('src/jobs/hooks.js');
const { runHook } = await load('src/hooks/runner.js');

const SCRIPT = join(stage, 'hooks', 'tag.mjs');
const hooks = new HookRegistry([{ name: 'tag', event: 'import.finished', script: SCRIPT }]);
const relay = createRelay({
  clock: () => new Date('2026-01-01T00:00:00Z'),
  hooks,
  transport: async (href) => ({ status: 200, href }),
});
const insecure = createRelay({
  clock: () => new Date('2026-01-01T00:00:00Z'),
  config: { allowInsecure: true },
  transport: async (href) => ({ status: 200, href }),
});

const userinfo = 'https://example.com@127.0.0.1:8080/hook';
const numeric = 'http://2130706433/latest/meta-data';

async function attempt(label, run) {
  try {
    return `${label} -> ${await run()}`;
  } catch (error) {
    return `${label} -> blocked ${error.message}`;
  }
}

// A sibling entry point validates with assertTarget before the row is stored.
console.log(
  await attempt(`ok Relay.enqueue('${userinfo}')`, async () => relay.enqueue(sampleDelivery({ url: userinfo })).id),
);
console.log(
  await attempt(`ok assertTarget('${numeric}')`, async () => insecure.enqueue(sampleDelivery({ url: numeric })).url),
);

// A sibling hook run validates its argument and spawns without a shell.
const guarded = await runHook(SCRIPT, ['payload.json']);
console.log(`ok runHook('payload.json') -> ${guarded.stdout}`);

// D1: deliverNow posts a stored row, and the webhook client checks the raw URL text.
const stored = relay.store.insert(sampleDelivery({ url: userinfo }));
console.log(
  await attempt(`D1 Relay.deliverNow({url: '${userinfo}'})`, async () => {
    const receipt = await relay.deliverNow(stored.id);
    return `posted to ${receipt.host}`;
  }),
);
console.log(
  await attempt(`D1 Relay.deliverNow({url: '${numeric}'})`, async () => {
    const row = insecure.store.insert(sampleDelivery({ url: numeric }));
    const receipt = await insecure.deliverNow(row.id);
    return `posted to ${receipt.host}`;
  }),
);

// D2: the import hook builds one shell command out of the hook and the payload name.
console.log(
  await attempt("D2 runPostImportHook('payload.json; printf relay-injected')", async () => {
    const result = await runPostImportHook(hooks, 'payload.json; printf relay-injected');
    return `stdout ${JSON.stringify(result.stdout)}`;
  }),
);

await rm(stage, { recursive: true, force: true });
