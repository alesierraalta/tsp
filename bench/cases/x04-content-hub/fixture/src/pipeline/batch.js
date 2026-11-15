import { mkdtemp, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { runTool } from './tools.js';
import { planVariants, fingerprint } from '../media/variants.js';
import { imageSize } from '../media/images.js';

export class RenderBatch {
  constructor(options = {}) {
    this.entries = options.entries;
    this.renderer = options.renderer;
    this.media = options.media;
    this.bus = options.bus;
    this.clock = options.clock ?? (() => new Date());
  }

  async html(entry) {
    return this.renderer.renderEntry(entry, { template: 'entry.html' });
  }

  async markdown(source) {
    const dir = await mkdtemp(join(tmpdir(), 'hub-markdown-'));
    const file = join(dir, 'entry.md');
    await writeFile(file, String(source ?? ''));
    try {
      const { stdout } = await runTool('markdown', [file]);
      return stdout.trim();
    } finally {
      await rm(dir, { recursive: true, force: true });
    }
  }

  async variants(name, buffer) {
    const size = imageSize(name, buffer);
    const version = fingerprint(buffer);
    return planVariants(name, size, ['thumb', 'card']).map((variant) => ({ ...variant, version }));
  }

  async run(entryIds, options = {}) {
    const out = [];
    for (const id of entryIds) {
      const entry = this.entries.get(id);
      const html = options.template
        ? await this.renderer.renderEntry(entry, { template: options.template })
        : await this.html(entry);
      if (this.bus) {
        await this.bus.emit('render.afterDocument', { entry, fragment: html });
      }
      out.push({ id, slug: entry.slug, bytes: Buffer.byteLength(html) });
    }
    return { rendered: out, total_bytes: out.reduce((total, item) => total + item.bytes, 0) };
  }

  async preview(entry, limit = 400) {
    const html = await this.html(entry);
    return html.slice(0, limit);
  }

  async assetTargets(files) {
    const out = [];
    for (const file of files) {
      const buffer = await this.media.read(file);
      out.push({ name: file, variants: await this.variants(file, buffer) });
    }
    return out;
  }
}

export function batchSummary(results) {
  return {
    entries: results.length,
    bytes: results.reduce((total, item) => total + (item.bytes ?? 0), 0),
    slugs: results.map((item) => item.slug),
  };
}
