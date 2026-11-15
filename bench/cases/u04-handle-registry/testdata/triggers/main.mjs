// Exercises the planted defects against one variant directory of this case:
//   node testdata/triggers/main.mjs fixture
//   node testdata/triggers/main.mjs fix/all
//   node testdata/triggers/main.mjs fix/keep-D1
// The output is what KEY.json records as trigger.actual.
import { pathToFileURL } from 'node:url';

const variant = process.argv[2] ?? 'fixture';
const { Registry } = await import(pathToFileURL(`${process.cwd()}/${variant}/src/handles.js`).href);

function attempt(run) {
  try {
    return `ok ${run()}`;
  } catch (error) {
    return `threw ${error.message}`;
  }
}

const decomposed = 'Zürich'.normalize('NFD');
const registry = new Registry();
registry.claim(decomposed, 'first owner');
const second = attempt(() => JSON.stringify(registry.claim('Zürich', 'impostor').handle));
const renders = registry.list().map((entry) => `@${entry.handle.normalize('NFC')}`).join(',');
console.log(`D1 claim(NFD 'Zürich') then claim(NFC 'Zürich') -> second=${second} entries=${registry.list().length} renders=${renders}`);

const turkish = new Registry({ locale: 'tr' });
const reserved = attempt(() => JSON.stringify(turkish.claim('ADMIN').handle));
console.log(`D2 new Registry({ locale: 'tr' }).claim('ADMIN') -> ${reserved} entries=${turkish.list().length}`);

const english = new Registry({ locale: 'en-US' });
console.log(`ok new Registry({ locale: 'en-US' }).claim('Admin') -> ${attempt(() => english.claim('Admin'))}`);
console.log(
  `ok new Registry().claim('avery') then claim('Avery') -> ${attempt(() => {
    const taken = new Registry();
    taken.claim('avery');
    return taken.claim('Avery');
  })}`,
);
