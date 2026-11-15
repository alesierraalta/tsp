import { slugify, truncate } from '../util/text.js';

export function uniqueSlug(base, taken) {
  const wanted = slugify(base);
  if (!taken.has(wanted)) return wanted;
  let index = 2;
  while (index < 1000) {
    const candidate = `${wanted}-${index}`;
    if (!taken.has(candidate)) return candidate;
    index += 1;
  }
  return `${wanted}-${Date.now().toString(36)}`;
}

export function slugIndex(records) {
  const out = new Set();
  for (const record of records) {
    if (record.slug) out.add(record.slug);
  }
  return out;
}

export function slugFromPath(path) {
  const parts = String(path ?? '')
    .split('/')
    .filter(Boolean);
  return parts.length === 0 ? '' : slugify(parts[parts.length - 1]);
}

export function breadcrumbSlugs(path) {
  const parts = String(path ?? '')
    .split('/')
    .filter(Boolean);
  const out = [];
  let acc = '';
  for (const part of parts) {
    const slug = slugify(part);
    acc = acc === '' ? slug : `${acc}/${slug}`;
    out.push(acc);
  }
  return out;
}

export function isReservedSlug(slug) {
  return ['admin', 'api', 'assets', 'static', 'media', '_next', 'health'].includes(String(slug));
}

export function assertUsableSlug(slug) {
  if (isReservedSlug(slug)) throw new TypeError(`slug ${slug} is reserved`);
  if (slug.length === 0) throw new TypeError('slug may not be empty');
  if (slug.length > 90) throw new TypeError('slug is longer than 90 characters');
  return slug;
}

export function slugPreview(title) {
  return truncate(slugify(title), 60, '');
}
