import { readFile, writeFile } from 'node:fs/promises';
import { runTool } from '../pipeline/tools.js';

export const TRANSFORM_FORMATS = {
  fragment: 'html fragment',
  document: 'standalone html document',
  text: 'plain text',
};

export function normalizeFormat(input) {
  const text = String(input ?? 'fragment').toLowerCase();
  return Object.prototype.hasOwnProperty.call(TRANSFORM_FORMATS, text) ? text : 'fragment';
}

export function transformSummary(plugin, entry) {
  return {
    plugin: plugin.name,
    entry: entry.slug,
    format: normalizeFormat(plugin.format),
  };
}

export async function runDeclaredTransform(plugin, entry, options = {}) {
  const format = normalizeFormat(plugin.format);
  const parts = [entry.slug, `--format=${format}`, `--template=${plugin.template ?? 'entry'}`];
  const { stdout } = await runTool('markdown', parts, { timeout: options.timeout ?? 20000 });
  return { output: String(stdout).trim(), format, plugin: plugin.name };
}

export async function writeTransformOutput(file, result) {
  if (!file) return result;
  await writeFile(file, `${result.output}\n`);
  return result;
}

export async function transformFile(plugin, entry, file) {
  const result = await runDeclaredTransform(plugin, entry);
  return writeTransformOutput(file, result);
}

export function supportedFormats() {
  return Object.keys(TRANSFORM_FORMATS);
}

export async function readRendered(file) {
  return readFile(file, 'utf8');
}
