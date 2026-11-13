#!/usr/bin/env node
import { readFile, stat, writeFile } from 'node:fs/promises';
import { isAbsolute, resolve } from 'node:path';

const [source, destination] = process.argv.slice(2);
const width = Number(process.argv.find((arg) => arg.startsWith('--width='))?.split('=')[1] ?? 0);

if (!source || !destination) {
  process.stderr.write('resize: a source and a destination are required\n');
  process.exit(2);
}
if (!Number.isInteger(width) || width <= 0 || width > 4096) {
  process.stderr.write('resize: --width must be a whole number between 1 and 4096\n');
  process.exit(2);
}

const read = (path) => resolve(isAbsolute(path) ? path : `${process.cwd()}/${path}`);
const info = await stat(read(source)).catch(() => undefined);
if (!info || !info.isFile()) {
  process.stderr.write(`resize: ${source} is not a file\n`);
  process.exit(2);
}

const body = await readFile(read(source));
if (body.length > 8 * 1024 * 1024) {
  process.stderr.write('resize: the source is larger than 8 MiB\n');
  process.exit(2);
}

const header = Buffer.from(`${width}x${Math.round(width * 0.75)}`);
await writeFile(read(destination), Buffer.concat([header, body]));
process.stdout.write(`${destination} ${header.toString('utf8')} ${body.length}\n`);
