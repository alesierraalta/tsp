#!/usr/bin/env node
import { readFile } from 'node:fs/promises';
import { isAbsolute, resolve } from 'node:path';

function parse(argv) {
  const out = { positional: [], flags: {} };
  for (const arg of argv) {
    if (arg.startsWith('--')) {
      const [key, value] = arg.slice(2).split('=');
      out.flags[key] = value === undefined ? 'true' : value;
      continue;
    }
    out.positional.push(arg);
  }
  return out;
}

function fail(message, code = 2) {
  process.stderr.write(`replay: ${message}\n`);
  process.exit(code);
}

const args = parse(process.argv.slice(2));
const file = args.positional[0];
if (!file) fail('a jsonl file is required');

const chunk = Number(args.flags.chunk ?? 500);
if (!Number.isInteger(chunk) || chunk < 1 || chunk > 5000) fail('--chunk must be a whole number between 1 and 5000');

const path = resolve(isAbsolute(file) ? file : `${process.cwd()}/${file}`);
const text = await readFile(path, 'utf8').catch(() => fail(`${file} cannot be read`));
const lines = text.split('\n').filter((line) => line.trim() !== '');
if (lines.length === 0) fail(`${file} holds no events`);

const rows = [];
const problems = [];
lines.forEach((line, index) => {
  try {
    const row = JSON.parse(line);
    if (typeof row.name !== 'string' || row.name === '') problems.push({ line: index + 1, reason: 'an event needs a name' });
    else rows.push(row);
  } catch (error) {
    problems.push({ line: index + 1, reason: `not valid json: ${error.message}` });
  }
});

const batches = Math.ceil(rows.length / chunk);
const summary = {
  file,
  events: rows.length,
  rejected: problems.length,
  batches,
  chunk,
  names: [...new Set(rows.map((row) => row.name))].sort(),
};

if (args.flags['dry-run'] === 'true') {
  process.stdout.write(`${JSON.stringify({ ...summary, dry_run: true }, null, 2)}\n`);
  process.exit(0);
}

process.stdout.write(`${JSON.stringify(summary, null, 2)}\n`);
if (problems.length > 0) process.exitCode = 1;
