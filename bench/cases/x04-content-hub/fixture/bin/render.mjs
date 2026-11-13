#!/usr/bin/env node
import { readFile } from 'node:fs/promises';
import { isAbsolute, join } from 'node:path';

const FORMATS = ['fragment', 'document', 'text'];
const TEMPLATES = ['entry', 'article-card', 'listing', 'markdown'];

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
  process.stderr.write(`render: ${message}\n`);
  process.exit(code);
}

const args = parse(process.argv.slice(2));
const format = args.flags.format ?? 'fragment';
const template = args.flags.template ?? 'entry';

if (!FORMATS.includes(format)) fail(`unknown format ${format}`);
if (!TEMPLATES.includes(template)) fail(`unknown template ${template}`);

const source = args.positional[0];
if (source === undefined) fail('an entry slug or file is required');

if (isAbsolute(source)) fail('an absolute path is not an entry slug');

const body = source.endsWith('.md') || source.endsWith('.json')
  ? await readFile(join(process.cwd(), source), 'utf8').catch(() => fail(`${source} cannot be read`))
  : '';

const escaped = body.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
const entry = source
  .replace(/&/g, '&amp;')
  .replace(/</g, '&lt;')
  .replace(/>/g, '&gt;')
  .replace(/"/g, '&quot;');
const parts = [`<article data-entry="${entry}" data-template="${template}" data-format="${format}">`];
if (escaped.trim() !== '') parts.push(escaped.trim());
parts.push('</article>');
const payload = format === 'text' ? escaped.trim() : parts.join('');
process.stdout.write(`${payload}\n`);
