import { mkdir, readdir, readFile, rename, rm, stat, writeFile } from 'node:fs/promises';
import { dirname, join } from 'node:path';
import { notFound, tooLarge, unsafePath } from '../http/errors.js';
import { assertPlainName, mimeOf, resolveUnder } from './paths.js';

export const MAX_DOCUMENT_BYTES = 1024 * 1024;

export class DocumentStore {
  constructor(options = {}) {
    this.root = options.root;
    if (!this.root) throw new TypeError('a document root is required');
    this.limit = options.limit ?? MAX_DOCUMENT_BYTES;
  }

  fileOf(name) {
    return resolveUnder(this.root, assertPlainName(name));
  }

  async list() {
    let entries;
    try {
      entries = await readdir(this.root, { withFileTypes: true });
    } catch (error) {
      if (error.code === 'ENOENT') return [];
      throw error;
    }
    return entries
      .filter((entry) => entry.isFile())
      .map((entry) => ({ name: entry.name, type: mimeOf(entry.name) }))
      .sort((left, right) => (left.name < right.name ? -1 : 1));
  }

  async read(name) {
    const file = this.fileOf(name);
    const info = await stat(file).catch(() => undefined);
    if (!info || !info.isFile()) throw notFound(`document ${name}`);
    if (info.size > this.limit) throw tooLarge(this.limit);
    const data = await readFile(file);
    return { name: assertPlainName(name), size: info.size, type: mimeOf(name), data };
  }

  async readText(name) {
    const document = await this.read(name);
    return document.data.toString('utf8');
  }

  async write(name, data) {
    const file = this.fileOf(name);
    const payload = Buffer.isBuffer(data) ? data : Buffer.from(String(data), 'utf8');
    if (payload.length > this.limit) throw tooLarge(this.limit);
    await mkdir(dirname(file), { recursive: true });
    const staging = join(dirname(file), `.${assertPlainName(name)}.${process.pid}.tmp`);
    await writeFile(staging, payload);
    await rename(staging, file);
    return { name: assertPlainName(name), size: payload.length, type: mimeOf(name) };
  }

  async append(name, text) {
    let current = '';
    try {
      current = await this.readText(name);
    } catch (error) {
      if (error.code !== 'not_found') throw error;
    }
    return this.write(name, `${current}${text}`);
  }

  async remove(name) {
    const file = this.fileOf(name);
    const info = await stat(file).catch(() => undefined);
    if (!info) throw notFound(`document ${name}`);
    await rm(file);
    return { name: assertPlainName(name), removed: true };
  }

  async describe(name) {
    const file = this.fileOf(name);
    const info = await stat(file).catch(() => undefined);
    if (!info) throw notFound(`document ${name}`);
    return {
      name: assertPlainName(name),
      size: info.size,
      modified: info.mtime.toISOString(),
      type: mimeOf(name),
    };
  }

  async usage() {
    const entries = await this.list();
    let total = 0;
    for (const entry of entries) {
      const info = await stat(resolveUnder(this.root, entry.name)).catch(() => undefined);
      if (info) total += info.size;
    }
    return { files: entries.length, bytes: total };
  }

  guard(name) {
    try {
      return this.fileOf(name);
    } catch (error) {
      if (error.status === 400) throw unsafePath(name);
      throw error;
    }
  }
}
