// Exercises the planted defects against one variant directory of this case:
//   node testdata/triggers/main.mjs fixture
//   node testdata/triggers/main.mjs fix/all
//   node testdata/triggers/main.mjs fix/keep-D1
// The output is what KEY.json records as trigger.actual.
import { pathToFileURL } from 'node:url';

const variant = process.argv[2] ?? 'fixture';
const { headerMap, merge } = await import(pathToFileURL(`${process.cwd()}/${variant}/src/envelope.js`).href);

function attempt(run) {
  try {
    return `ok ${run()}`;
  } catch (error) {
    return `threw ${error.message}`;
  }
}

const headers = attempt(() => JSON.stringify(headerMap(['Set-Cookie: session=a', 'set-cookie: theme=dark'])));
console.log(`D2 headerMap(['Set-Cookie: session=a','set-cookie: theme=dark']) -> ${headers}`);

// The pollution check runs last: on the fixture it changes this process's Object.prototype.
const before = String({}.polluted);
let envelope;
try {
  envelope = JSON.parse('{"__proto__":{"polluted":"yes"}}');
} catch (error) {
  console.log(`D1 could not build the envelope: ${error.message}`);
}
const merged = attempt(() => JSON.stringify(merge({}, envelope)));
console.log(`D1 merge({}, {"__proto__":{"polluted":"yes"}}) -> ${merged} ({}).polluted: ${before} -> ${String({}.polluted)}`);

console.log(`ok headerMap(['Accept: text/html']) -> ${attempt(() => JSON.stringify(headerMap(['Accept: text/html'])))}`);
