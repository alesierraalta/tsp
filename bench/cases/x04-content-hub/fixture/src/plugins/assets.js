import { readFile, readdir, stat, writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import { notFound, unsafePath } from '../util/errors.js';
import { assertPlainName, extensionOf, mimeOf, resolveUnder } from '../media/paths.js';

const MAX_BYTES = 512 * 1024;

export class PluginAssetStore {
  constructor(options = {}) {
    this.root = options.root;
    if (!this.root) throw new TypeError('a plugin asset root is required');
    this.clock = options.clock ?? (() => new Date());
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

  async write(name, body) {
    const clean = assertPlainName(name);
    const payload = Buffer.isBuffer(body) ? body : Buffer.from(String(body ?? ''), 'utf8');
    if (payload.length > MAX_BYTES) throw new TypeError(`a plugin asset holds at most ${MAX_BYTES} bytes`);
    const file = resolveUnder(this.root, clean);
    await writeFile(file, payload);
    return { name: clean, bytes: payload.length, written_at: new Date(this.clock()).toISOString() };
  }

  async describe(name) {
    const clean = assertPlainName(name);
    const file = resolveUnder(this.root, clean);
    const info = await stat(file).catch(() => undefined);
    if (!info || !info.isFile()) throw notFound(`plugin asset ${name}`);
    return { name: clean, bytes: info.size, type: mimeOf(clean) };
  }

  async read(name) {
    const clean = String(name ?? '');
    if (clean.length === 0) throw unsafePath(name);
    const file = join(this.root, clean);
    const info = await stat(file).catch(() => undefined);
    if (!info || !info.isFile()) throw notFound(`plugin asset ${name}`);
    const body = await readFile(file);
    return { name: clean, type: mimeOf(clean.split('/').pop()), body };
  }

  async readText(name) {
    return (await this.read(name)).body.toString('utf8');
  }

  async remove(name) {
    const clean = assertPlainName(name);
    const file = resolveUnder(this.root, clean);
    const info = await stat(file).catch(() => undefined);
    if (!info || !info.isFile()) throw notFound(`plugin asset ${name}`);
    await writeFile(file, '');
    return { name: clean, cleared: true };
  }

  async byExtension(extension) {
    const wanted = String(extension ?? '').toLowerCase();
    return (await this.list()).filter((entry) => extensionOf(entry.name) === wanted);
  }
}
