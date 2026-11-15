import { createHash } from 'node:crypto';
import { mkdir, readFile, readdir, stat, unlink, writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import { notFound, tooLarge, invalidField } from '../util/errors.js';
import { assertPlainName, extensionOf, isImage, mimeOf, resolveUnder } from './paths.js';

const MAX_BYTES = 1024 * 1024;

export class MediaStore {
  constructor(options = {}) {
    this.root = options.root;
    if (!this.root) throw new TypeError('a media root is required');
    this.clock = options.clock ?? (() => new Date());
    this.maxBytes = options.maxBytes ?? MAX_BYTES;
    this.described = new Map();
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
      .map((entry) => ({ name: entry.name, type: mimeOf(entry.name), image: isImage(entry.name) }))
      .sort((left, right) => (left.name < right.name ? -1 : 1));
  }

  async put(name, body) {
    const clean = assertPlainName(name);
    const payload = Buffer.isBuffer(body) ? body : Buffer.from(String(body ?? ''), 'utf8');
    if (payload.length > this.maxBytes) {
      throw tooLarge(`a media file holds at most ${this.maxBytes} bytes`);
    }
    if (payload.length === 0) throw invalidField('body', 'is empty');
    await mkdir(this.root, { recursive: true });
    const file = resolveUnder(this.root, clean);
    await writeFile(file, payload);
    const described = {
      name: clean,
      bytes: payload.length,
      type: mimeOf(clean),
      stored_at: new Date(this.clock()).toISOString(),
    };
    this.described.set(clean, described);
    return described;
  }

  async read(name) {
    const clean = assertPlainName(name);
    const file = resolveUnder(this.root, clean);
    const info = await stat(file).catch(() => undefined);
    if (!info || !info.isFile()) throw notFound(`media file ${name}`);
    return readFile(file);
  }

  async readText(name) {
    return (await this.read(name)).toString('utf8');
  }

  async describe(name) {
    const clean = assertPlainName(name);
    const file = resolveUnder(this.root, clean);
    const info = await stat(file).catch(() => undefined);
    if (!info || !info.isFile()) throw notFound(`media file ${name}`);
    return {
      name: clean,
      bytes: info.size,
      type: mimeOf(clean),
      image: isImage(clean),
      stored_at: this.described.get(clean)?.stored_at ?? info.mtime.toISOString(),
    };
  }

  async remove(name) {
    const clean = assertPlainName(name);
    const file = resolveUnder(this.root, clean);
    const info = await stat(file).catch(() => undefined);
    if (!info || !info.isFile()) throw notFound(`media file ${name}`);
    await unlink(file);
    this.described.delete(clean);
    return { name: clean, removed: true };
  }

  async exists(name) {
    try {
      await this.describe(name);
      return true;
    } catch {
      return false;
    }
  }

  async listByExtension(extension) {
    const wanted = String(extension ?? '').toLowerCase();
    const entries = await this.list();
    return entries.filter((entry) => extensionOf(entry.name) === wanted);
  }

  async totalBytes() {
    const entries = await this.list();
    let total = 0;
    for (const entry of entries) total += (await this.describe(entry.name)).bytes;
    return total;
  }

  pathOf(name) {
    return join(this.root, assertPlainName(name));
  }

  async checksumOf(name) {
    const body = await this.read(name);
    return createHash('sha256').update(body).digest('hex');
  }

  async dedupe() {
    const entries = await this.list();
    const seen = new Map();
    const groups = [];
    for (const entry of entries) {
      const checksum = await this.checksumOf(entry.name);
      const existing = seen.get(checksum);
      if (existing) {
        existing.push(entry.name);
        continue;
      }
      seen.set(checksum, [entry.name]);
    }
    for (const [checksum, names] of seen.entries()) {
      if (names.length > 1) groups.push({ checksum, names });
    }
    return groups;
  }

  async copy(from, to) {
    const body = await this.read(from);
    return this.put(to, body);
  }

  async rename(from, to) {
    const described = await this.copy(from, to);
    await this.remove(from);
    return described;
  }

  async usageIndex(entries) {
    const index = new Map();
    for (const entry of entries ?? []) {
      for (const [field, value] of Object.entries(entry.values ?? {})) {
        if (typeof value !== 'string' || !value.includes('.')) continue;
        const bucket = index.get(value);
        const reference = { entry: entry.id, field };
        if (bucket) bucket.push(reference);
        else index.set(value, [reference]);
      }
    }
    return index;
  }

  async unused(entries) {
    const index = await this.usageIndex(entries);
    const listed = await this.list();
    return listed.filter((entry) => !index.has(entry.name)).map((entry) => entry.name);
  }

  async largest(limit = 5) {
    const entries = await this.list();
    const described = [];
    for (const entry of entries) described.push(await this.describe(entry.name));
    return described.sort((left, right) => right.bytes - left.bytes).slice(0, limit);
  }
}
