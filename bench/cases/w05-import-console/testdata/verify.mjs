// Verifies this case the way the bench does: the happy suite is green on fixture/ and on every
// fix/ overlay (overlaid onto a copy of fixture/), and every trigger prints its recorded line.
//   node testdata/verify.mjs
import { spawn } from 'node:child_process';
import { cp, mkdtemp, readdir, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';

const caseDir = resolve(process.cwd());
const fixture = join(caseDir, 'fixture');
const fixDir = join(caseDir, 'fix');

function run(command, args, options = {}) {
  return new Promise((resolveRun) => {
    const child = spawn(command, args, { ...options, stdio: ['ignore', 'pipe', 'pipe'] });
    let output = '';
    child.stdout.on('data', (chunk) => {
      output += chunk;
    });
    child.stderr.on('data', (chunk) => {
      output += chunk;
    });
    child.on('close', (code) => resolveRun({ code, output }));
  });
}

let failures = 0;

const variants = [''];
const entries = await readdir(fixDir, { withFileTypes: true }).catch(() => []);
for (const entry of entries) {
  if (entry.isDirectory()) variants.push(join('fix', entry.name));
}

for (const variant of variants) {
  const work = await mkdtemp(join(tmpdir(), 'case-suite-'));
  try {
    await cp(fixture, work, { recursive: true });
    if (variant) await cp(join(caseDir, variant), work, { recursive: true });
    const { code, output } = await run('node', ['--test'], { cwd: work, timeout: 240000 });
    const summary = output
      .split('\n')
      .filter((line) => /^ℹ (tests|pass|fail)/.test(line))
      .join(' ');
    const label = variant === '' ? 'fixture' : variant;
    console.log(`suite ${label}: exit ${code} ${summary}`);
    if (code !== 0) {
      failures += 1;
      console.log(output.split('\n').slice(-40).join('\n'));
    }
  } finally {
    await rm(work, { recursive: true, force: true });
  }
}

for (const variant of variants) {
  const label = variant === '' ? 'fixture' : variant;
  const { code, output } = await run('node', ['testdata/triggers/main.mjs', label], {
    cwd: caseDir,
    timeout: 120000,
  });
  console.log(`trigger ${label}: exit ${code}`);
  console.log(output.trimEnd());
  if (code !== 0) failures += 1;
}

if (failures > 0) {
  console.log(`FAILED: ${failures} variant(s)`);
  process.exitCode = 1;
} else {
  console.log('all variants green and every trigger ran');
}
