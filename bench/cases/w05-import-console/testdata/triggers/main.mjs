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

const stage = await mkdtemp(join(tmpdir(), 'w05-stage-'));
await cp(join(caseDir, 'fixture'), stage, { recursive: true });
if (variant !== 'fixture') {
  await cp(join(caseDir, variant), stage, { recursive: true });
}
const load = (rel) => import(pathToFileURL(join(stage, rel)).href);

const { loadDataset } = await load('src/etl/loader.js');
const { selectStatement } = await load('src/export/sql.js');
const { quoteLiteral, searchClause } = await load('src/query/builder.js');
const { renderTable } = await load('src/render/components/table.js');
const { renderCard } = await load('src/render/components/card.js');

const CSV = [
  'sku,title,category,amount,stock,updated_at',
  'SKU-1,Desk lamp,lighting,120.50,4,2026-01-05',
].join('\n');
const { dataset } = loadDataset('items', CSV);

function tagOf(html, selector) {
  const match = html.match(selector);
  return match ? match[0] : html;
}

// A sibling filter in the same exporter goes through quoteLiteral.
console.log(
  `ok selectStatement('items', ['sku'], {filters:[{column:'sku',value:"A'1"}]}) -> ${selectStatement(
    'items',
    ['sku'],
    { filters: [{ column: 'sku', value: "A'1" }] },
  )}`,
);

// The guarded search helper in the query builder doubles the quote too.
console.log(`ok searchClause('title', "x'") -> ${searchClause('title', "x'")}`);
console.log(`ok quoteLiteral("x'") -> ${quoteLiteral("x'")}`);

// A card link escapes its attribute context.
const card = renderCard({ title: 'Site', body: 'link', href: 'https://example.com/a?b=" onmouseover="alert(1)' });
console.log(`ok renderCard({href: '...a?b=" onmouseover="alert(1)'}) -> ${tagOf(card, /<a class="card"[^>]*>/)}`);

// D1: the table link cell escapes its href with the text escaper.
const table = renderTable({
  columns: [{ key: 'site', label: 'Site', link: true }],
  rows: [{ site: 'https://example.com/a?b=" onmouseover="alert(1)' }],
});
console.log(`D1 renderTable({link:true}, site: '...a?b=" onmouseover="alert(1)') -> ${tagOf(table, /<a [^>]*>/)}`);

// D2: this exporter builds the search pattern into the SQL text itself.
console.log(
  `D2 selectStatement('items', ['sku'], {filters:[{op:'search',column:'title',value:"x' OR '1'='1"}]}) -> ${selectStatement(
    'items',
    ['sku'],
    { filters: [{ op: 'search', column: 'title', value: "x' OR '1'='1" }] },
  )}`,
);

// The data itself is still what a happy path exercises.
console.log(`ok dataset row -> ${JSON.stringify(dataset.get('SKU-1'))}`);

await rm(stage, { recursive: true, force: true });
