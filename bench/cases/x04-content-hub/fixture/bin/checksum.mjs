#!/usr/bin/env node
import { createHash } from 'node:crypto';
import { readFile, stat } from 'node:fs/promises';
import { isAbsolute, resolve } from 'node:path';

const target = process.argv[2];
if (!target) {
  process.stderr.write('checksum: a file path is required\n');
  process.exit(2);
}

const file = resolve(isAbsolute(target) ? target : `${process.cwd()}/${target}`);
const info = await stat(file).catch(() => undefined);
if (!info || !info.isFile()) {
  process.stderr.write(`checksum: ${target} is not a file\n`);
  process.exit(2);
}

const body = await readFile(file);
const digest = createHash('sha256').update(body).digest('hex');
process.stdout.write(`${digest}  ${target}  ${body.length}\n`);
