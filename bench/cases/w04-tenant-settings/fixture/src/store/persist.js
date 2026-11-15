import { mkdir, readdir, readFile, rename, rm, writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import { resolveUnder } from '../files/paths.js';
import { projectRow, validateRow } from './query.js';

export class SnapshotStore {
  constructor(options = {}) {
    this.dir = options.dir;
    if (!this.dir) throw new TypeError('a snapshot directory is required');
    this.clock = options.clock ?? (() => new Date());
  }

  fileOf(collection) {
    return resolveUnder(this.dir, `${collection}.json`);
  }

  async names() {
    let entries;
    try {
      entries = await readdir(this.dir, { withFileTypes: true });
    } catch (error) {
      if (error.code === 'ENOENT') return [];
      throw error;
    }
    return entries
      .filter((entry) => entry.isFile() && entry.name.endsWith('.json'))
      .map((entry) => entry.name.slice(0, -'.json'.length))
      .filter((name) => /^[a-z][a-z0-9_]*$/.test(name))
      .sort();
  }

  async save(collection, rows) {
    const file = this.fileOf(collection);
    const payload = rows.map((row) => validateRow(collection, projectRow(collection, row)));
    const body = JSON.stringify(
      { collection, written_at: new Date(this.clock()).toISOString(), rows: payload },
      null,
      2,
    );
    await mkdir(this.dir, { recursive: true });
    const staging = join(this.dir, `.${collection}.${process.pid}.tmp`);
    await writeFile(staging, `${body}\n`);
    await rename(staging, file);
    return { file, rows: payload.length, bytes: Buffer.byteLength(body) };
  }

  async load(collection) {
    const file = this.fileOf(collection);
    let raw;
    try {
      raw = await readFile(file, 'utf8');
    } catch (error) {
      if (error.code === 'ENOENT') return { rows: [], written_at: null };
      throw error;
    }
    let parsed;
    try {
      parsed = JSON.parse(raw);
    } catch (error) {
      throw new TypeError(`${collection}.json is not valid JSON: ${error.message}`);
    }
    if (!Array.isArray(parsed.rows)) {
      throw new TypeError(`${collection}.json does not hold rows`);
    }
    return { rows: parsed.rows, written_at: parsed.written_at ?? null };
  }

  async restore(store, collection) {
    const loaded = await this.load(collection);
    store.clear(collection);
    for (const row of loaded.rows) {
      store.insert(collection, row);
    }
    return loaded.rows.length;
  }

  async drop(collection) {
    const file = this.fileOf(collection);
    await rm(file, { force: true });
    return file;
  }

  async snapshot(store) {
    const written = [];
    for (const name of store.names()) {
      const result = await this.save(name, store.all(name));
      written.push({ collection: name, rows: result.rows, bytes: result.bytes });
    }
    return written;
  }

  async prune(keep = 5) {
    const names = await this.names();
    const dropped = [];
    for (const name of names.slice(0, Math.max(0, names.length - keep))) {
      await this.drop(name);
      dropped.push(name);
    }
    return dropped;
  }
}
