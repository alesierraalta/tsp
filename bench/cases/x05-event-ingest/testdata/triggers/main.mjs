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

const stage = await mkdtemp(join(tmpdir(), 'x05-stage-'));
await cp(join(caseDir, 'fixture'), stage, { recursive: true });
if (variant !== 'fixture') {
  await cp(join(caseDir, variant), stage, { recursive: true });
}

const load = (rel) => import(pathToFileURL(join(stage, rel)).href);
const { PlanBook } = await load('src/quota/plans.js');
const { ReservationBook } = await load('src/quota/reserve.js');
const { Invoice } = await load('src/billing/invoice.js');
const { sumCents } = await load('src/billing/rates.js');
const { StatementBook } = await load('src/billing/statements.js');

const clock = () => new Date('2026-02-10T12:00:00.000Z');
const plans = new PlanBook();
plans.assign('acme', 'growth');
const reservations = new ReservationBook({ plans, clock });

async function attempt(label, run) {
  try {
    return `${label} -> ${await run()}`;
  } catch (error) {
    return `${label} -> blocked ${error.message}`;
  }
}

// reserve() is the entry point that checks both numbers against the plan before it holds any slot.
console.log(
  await attempt("ok ReservationBook().reserve('acme', {rows: 10, samples: 100})", async () => {
    const reservation = reservations.reserve('acme', { rows: 10, samples: 100 });
    return `reserved ${reservation.slots} slots on the ${reservation.plan} plan`;
  }),
);

// D1: this entry point checks rows against the plan and sizes its slot array from the samples it was named.
console.log(
  await attempt("D1 ReservationBook().planCapacity('acme', {rows: 10, samples: 900000})", async () => {
    const reservation = reservations.planCapacity('acme', { rows: 10, samples: 900000 });
    return `reserved ${reservation.slots} slots on the ${reservation.plan} plan`;
  }),
);

// The rate card adds up a document's lines in whole cents, one rounding per line.
const lines = [
  { metric: 'samples', units: 5 },
  { metric: 'samples', units: 5 },
];
console.log(`ok sumCents(two 5-sample lines) -> ${sumCents(lines)} cents`);
const statement = new StatementBook({ clock }).build('acme', '2026-02', lines);
console.log(`ok StatementBook().build(two 5-sample lines) -> ${statement.total_cents} cents`);

// D2: a document accumulates its lines in dollars and truncates the total once.
const invoice = new Invoice({ tenant: 'acme', clock });
for (const line of lines) invoice.addLine(line);
const document = invoice.document();
console.log(`D2 Invoice(two 5-sample lines).total_cents -> ${document.total_cents} (its lines sum to ${invoice.check().lines})`);

await rm(stage, { recursive: true, force: true });
