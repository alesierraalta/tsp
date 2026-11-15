import { cliTheme, PREVIEWS } from './preview.js';

export const BIN_DIR = new URL('../../bin/', import.meta.url);

export function parseArgs(argv) {
  const out = { flags: {}, positional: [] };
  for (const arg of argv) {
    if (arg.startsWith('--')) {
      const [key, value] = arg.slice(2).split('=');
      out.flags[key] = value === undefined ? true : value;
      continue;
    }
    out.positional.push(arg);
  }
  return out;
}

export function usageExamples() {
  return [
    'node bin/render.mjs article/hello --format=document',
    'node bin/render.mjs --template=entry --format=fragment article/hello',
    'node bin/checksum.mjs content/article.json',
  ];
}

export function describePreview(name) {
  return { name, theme: cliTheme(), previews: PREVIEWS.length };
}
