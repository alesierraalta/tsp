// Exercises the planted defects against one variant of this case. The variant is staged from a copy of
// fixture/ overlaid with the named fix directory, because fix/ holds only the corrected files:
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

const stage = await mkdtemp(join(tmpdir(), 'x06-stage-'));
await cp(join(caseDir, 'fixture'), stage, { recursive: true });
if (variant !== 'fixture') {
  await cp(join(caseDir, variant), stage, { recursive: true });
}

const load = (rel) => import(pathToFileURL(join(stage, rel)).href);
const { assertTarget } = await load('src/webhooks/targets.js');
const { SubscriptionBook } = await load('src/webhooks/subscriptions.js');
const { WebhookSender, recordingTransport } = await load('src/webhooks/sender.js');
const { renderServiceTag, renderEnvironmentTag } = await load('src/views/partials.js');

const clock = () => new Date('2026-02-10T12:00:00.000Z');

async function attempt(label, run) {
  try {
    return `${label} -> ${await run()}`;
  } catch (error) {
    return `${label} -> blocked ${error.message}`;
  }
}

// assertTarget is the guard every other path uses: it reads the host the URL actually reaches.
console.log(await attempt("ok assertTarget('https://hooks.partner.example/relay')", async () => `allowed ${assertTarget('https://hooks.partner.example/relay').host}`));

// D1: this delivery path checks the url text the subscription was stored with, not the host it reaches.
const subscriptions = new SubscriptionBook({ clock });
const stored = subscriptions.restore({
  url: 'https://hooks.example.com@127.0.0.1:9000/relay',
  events: ['release.created'],
  tenant: 'acme',
});
const recorder = recordingTransport();
const sender = new WebhookSender({ clock, transport: recorder.transport });
console.log(
  await attempt("D1 WebhookSender().send(stored subscription url)", async () => {
    const result = await sender.send({ url: stored.url, payload: { event: 'release.created' }, subscription: stored.id, secret: 'whsec_test' });
    return `posted to ${result.host}`;
  }),
);

// attrPair is what the sibling partials use: it escapes the quote that would close the attribute.
const label = 'prod" onmouseover="alert(1)';
console.log(`ok renderServiceTag({name: '${label}'}) -> ${renderServiceTag({ name: label })}`);

// D2: this dashboard partial writes the stored label into its attribute with the text escaper.
console.log(`D2 renderEnvironmentTag({label: '${label}'}) -> ${renderEnvironmentTag({ name: 'production', label })}`);

await rm(stage, { recursive: true, force: true });
