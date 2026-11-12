// Exercises the planted defects against one variant directory of this case:
//   node testdata/triggers/main.mjs fixture
//   node testdata/triggers/main.mjs fix/all
//   node testdata/triggers/main.mjs fix/keep-D1
// The output is what KEY.json records as trigger.actual.
import { pathToFileURL } from 'node:url';

const variant = process.argv[2] ?? 'fixture';
const { Allowlist } = await import(pathToFileURL(`${process.cwd()}/${variant}/src/egress.js`).href);

function attempt(run) {
  try {
    return `admitted ${run()}`;
  } catch (error) {
    return `blocked ${error.message}`;
  }
}

const listed = new Allowlist({ hosts: ['example-cdn.com'] });
const lookalike = attempt(() => listed.check('https://notexample-cdn.com/track.gif').hostname);
console.log(`D1 Allowlist({hosts:['example-cdn.com']}).check('https://notexample-cdn.com/track.gif') -> ${lookalike}`);

const open = new Allowlist();
const userinfo = attempt(() => open.check('https://example-cdn.com@127.0.0.1:8080/admin').hostname);
const numeric = attempt(() => new Allowlist({ allowHttp: true }).check('http://2130706433/latest/meta-data').hostname);
console.log(`D2 Allowlist().check('https://example-cdn.com@127.0.0.1:8080/admin') -> ${userinfo}`);
console.log(`D2 Allowlist({allowHttp:true}).check('http://2130706433/latest/meta-data') -> ${numeric}`);

console.log(`ok Allowlist({hosts:['example-cdn.com']}).check('https://example-cdn.com/a.png') -> ${attempt(() => listed.check('https://example-cdn.com/a.png').hostname)}`);
console.log(`ok Allowlist().check('https://127.0.0.1/admin') -> ${attempt(() => open.check('https://127.0.0.1/admin').hostname)}`);
