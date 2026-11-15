// Exercises the planted defects against one variant directory of this case:
//   node testdata/triggers/main.mjs fixture
//   node testdata/triggers/main.mjs fix/all
//   node testdata/triggers/main.mjs fix/keep-D1
// The output is what KEY.json records as trigger.actual.
import { pathToFileURL } from 'node:url';

const variant = process.argv[2] ?? 'fixture';
const module_ = await import(pathToFileURL(`${process.cwd()}/${variant}/src/report.js`).href);

const d1 = module_.attr('title', 'Total" onmouseover="alert(1)');
console.log(`D1 attr('title', 'Total" onmouseover="alert(1)') -> ${d1}`);

const d2 = module_.renderRich('<b>Restock</b><script>alert(1)</script>');
console.log(`D2 renderRich('<b>Restock</b><script>alert(1)</script>') -> ${d2}`);

const ok = module_.escapeText('<b>');
console.log(`ok escapeText('<b>') -> ${ok}`);
