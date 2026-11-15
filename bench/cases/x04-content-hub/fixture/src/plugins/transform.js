import { readFile, writeFile } from 'node:fs/promises';
import { exec } from 'node:child_process';
import { promisify } from 'node:util';
import { join } from 'node:path';
import { TOOLS } from '../pipeline/tools.js';

const run = promisify(exec);

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

export function toolScriptPath(toolDir) {
  return join(toolDir, TOOLS.markdown.file);
}

export async function runDeclaredTransform(plugin, entry, options = {}) {
  const format = normalizeFormat(plugin.format);
  const toolDir = options.toolDir ?? join(process.cwd(), 'bin');
  const parts = [entry.slug, `--format=${format}`, `--template=${plugin.template ?? 'entry'}`];
  const command = `${process.execPath} ${toolScriptPath(toolDir)} ${parts.join(' ')}`;
  const { stdout } = await run(command, { timeout: options.timeout ?? 20000 });
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
