import { mkdir, readFile, readdir, stat, writeFile } from 'node:fs/promises';
import { dirname } from 'node:path';
import { notFound } from '../http/errors.js';
import { assertPlainName, resolveUnder } from '../files/paths.js';

export class ReportCatalog {
  constructor(options = {}) {
    this.root = options.root;
    if (!this.root) throw new TypeError('a catalog root is required');
  }

  fileOf(name) {
    return resolveUnder(this.root, assertPlainName(name));
  }

  async index() {
    const file = resolveUnder(this.root, 'index.json');
    const raw = await readFile(file, 'utf8');
    let parsed;
    try {
      parsed = JSON.parse(raw);
    } catch (error) {
      throw new TypeError(`index.json is not valid JSON: ${error.message}`);
    }
    if (!Array.isArray(parsed.reports)) {
      throw new TypeError('index.json does not list reports');
    }
    return parsed.reports;
  }

  async entries() {
    let names;
    try {
      names = await readdir(this.root, { withFileTypes: true });
    } catch (error) {
      if (error.code === 'ENOENT') return [];
      throw error;
    }
    return names
      .filter((entry) => entry.isFile() && entry.name.endsWith('.md'))
      .map((entry) => entry.name.replace(/\.md$/, ''))
      .sort();
  }

  async read(name) {
    const file = this.fileOf(`${name}.md`);
    const info = await stat(file).catch(() => undefined);
    if (!info) throw notFound(`report ${name}`);
    const body = await readFile(file, 'utf8');
    return { name, body, bytes: info.size };
  }

  async write(name, body) {
    const file = this.fileOf(`${name}.md`);
    await mkdir(dirname(file), { recursive: true });
    await writeFile(file, String(body));
    return { name, bytes: Buffer.byteLength(String(body)) };
  }

  async exists(name) {
    const file = this.fileOf(`${name}.md`);
    const info = await stat(file).catch(() => undefined);
    return Boolean(info?.isFile());
  }

  async describe(name) {
    const file = this.fileOf(`${name}.md`);
    const info = await stat(file).catch(() => undefined);
    if (!info) throw notFound(`report ${name}`);
    return { name, bytes: info.size, modified: info.mtime.toISOString() };
  }
}
