#!/usr/bin/env node
import { readFile } from 'node:fs/promises';
import { isAbsolute, resolve } from 'node:path';
import { releaseNotes, notesMarkdown, notesText, notesSummary } from '../src/reports/notes.js';
import { releaseSummary } from '../src/reports/summary.js';
import { metricTable } from '../src/reports/metrics.js';

function parse(argv) {
  const out = { positional: [], flags: {} };
  for (const arg of argv) {
    if (arg.startsWith('--')) {
      const [name, value] = arg.slice(2).split('=');
      out.flags[name] = value === undefined ? 'true' : value;
      continue;
    }
    out.positional.push(arg);
  }
  return out;
}

function fail(message, code = 2) {
  process.stderr.write(`report: ${message}\n`);
  process.exit(code);
}

const args = parse(process.argv.slice(2));
const file = args.positional[0];
if (!file) fail('a release file is required');

const path = resolve(isAbsolute(file) ? file : `${process.cwd()}/${file}`);
const text = await readFile(path, 'utf8').catch(() => fail(`${file} cannot be read`));

let document;
try {
  document = JSON.parse(text);
} catch (error) {
  fail(`${file} is not valid JSON: ${error.message}`);
}

const release = document.release ?? document;
const format = String(args.flags.format ?? 'text');
if (!['text', 'markdown', 'json'].includes(format)) fail('--format is text, markdown or json');

const notes = releaseNotes(release, { at: args.flags.at });
const deployments = Array.isArray(document.deployments) ? document.deployments : [];
const incidents = Array.isArray(document.incidents) ? document.incidents : [];

if (format === 'json') {
  process.stdout.write(
    `${JSON.stringify({ notes: notesSummary(notes), summary: releaseSummary([release]), metrics: metricTable(deployments, incidents) }, null, 2)}\n`,
  );
  process.exit(0);
}

if (format === 'markdown') {
  process.stdout.write(`${notesMarkdown(notes, { footer: args.flags.footer })}\n`);
  process.exit(0);
}

process.stdout.write(`${notesText(notes, { footer: args.flags.footer })}\n`);
