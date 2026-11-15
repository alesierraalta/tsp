const DIACRITICS = /[\u0300-\u036f]/g;

export function normalizeWhitespace(text) {
  return String(text ?? '')
    .replace(/\r\n/g, '\n')
    .replace(/[ \t]+/g, ' ')
    .replace(/ *\n */g, '\n')
    .trim();
}

export function singleLine(text) {
  return normalizeWhitespace(text).replace(/\n+/g, ' ');
}

export function slugify(text) {
  const folded = String(text ?? '')
    .normalize('NFKD')
    .replace(DIACRITICS, '')
    .toLowerCase();
  const dashed = folded.replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '');
  return dashed.slice(0, 80) || 'untitled';
}

export function truncate(text, limit = 120, ellipsis = '…') {
  const value = String(text ?? '');
  if (value.length <= limit) return value;
  return value.slice(0, Math.max(0, limit - ellipsis.length)) + ellipsis;
}

export function titleCase(text) {
  return singleLine(text)
    .split(' ')
    .map((word) => (word.length === 0 ? word : word[0].toUpperCase() + word.slice(1).toLowerCase()))
    .join(' ');
}

export function capitalize(text) {
  const value = String(text ?? '');
  return value.length === 0 ? value : value[0].toUpperCase() + value.slice(1);
}

export function kebabCase(text) {
  return String(text ?? '')
    .replace(/([a-z0-9])([A-Z])/g, '$1-$2')
    .replace(/[\s_]+/g, '-')
    .toLowerCase();
}

export function camelCase(text) {
  const parts = String(text ?? '')
    .split(/[^A-Za-z0-9]+/)
    .filter(Boolean);
  return parts
    .map((part, index) => (index === 0 ? part.toLowerCase() : capitalize(part.toLowerCase())))
    .join('');
}

export function splitCsv(text, separator = ',') {
  const value = String(text ?? '');
  if (value.trim() === '') return [];
  return value
    .split(separator)
    .map((part) => part.trim())
    .filter((part) => part.length > 0);
}

export function pluralize(count, singular, plural) {
  const word = count === 1 ? singular : (plural ?? `${singular}s`);
  return `${count} ${word}`;
}

export function pad(text, width, fill = ' ') {
  const value = String(text ?? '');
  if (value.length >= width) return value;
  return value + fill.repeat(width - value.length);
}

export function isBlank(value) {
  return value === null || value === undefined || String(value).trim() === '';
}

export function excerpt(text, words = 24) {
  const parts = singleLine(text).split(' ').filter(Boolean);
  if (parts.length <= words) return parts.join(' ');
  return `${parts.slice(0, words).join(' ')}…`;
}

export function stripTags(text) {
  return singleLine(String(text ?? '').replace(/<[^>]*>/g, ' '));
}

export function wordCount(text) {
  return singleLine(text).split(' ').filter(Boolean).length;
}

export function containsAny(haystack, needles) {
  const value = String(haystack ?? '').toLowerCase();
  return needles.some((needle) => value.includes(String(needle).toLowerCase()));
}

export function initials(name, limit = 2) {
  const parts = singleLine(name).split(' ').filter(Boolean);
  return parts.slice(0, limit).map((part) => part[0].toUpperCase()).join('');
}

export function maskEmail(email) {
  const text = String(email ?? '');
  const at = text.indexOf('@');
  if (at <= 0) return text;
  const local = text.slice(0, at);
  const domain = text.slice(at + 1);
  const visible = local.slice(0, Math.min(2, local.length));
  return `${visible}${'*'.repeat(Math.max(1, local.length - visible.length))}@${domain}`;
}

export function readingTime(text, wordsPerMinute = 220) {
  const words = wordCount(stripTags(text));
  return Math.max(1, Math.round(words / Math.max(1, wordsPerMinute)));
}

export function headingsOf(markdown) {
  return String(markdown ?? '')
    .split('\n')
    .map((line) => /^(#{1,6})\s+(.*)$/.exec(line.trim()))
    .filter(Boolean)
    .map((match) => ({ level: match[1].length, text: match[2].trim() }));
}

export function slugifyHeading(text) {
  return slugify(text).slice(0, 60);
}

export function linkify(text, base = '') {
  return String(text ?? '').replace(/(^|\s)(@[a-z0-9._-]{2,30})/gi, (whole, space, handle) => {
    const name = handle.slice(1).toLowerCase();
    return `${space}<a href="${base}/${name}" rel="mention">@${name}</a>`;
  });
}

export function paragraphsOf(text) {
  return normalizeWhitespace(text)
    .split(/\n{2,}/)
    .map((part) => part.trim())
    .filter(Boolean);
}

export function sentenceCount(text) {
  return singleLine(text)
    .split(/(?<=[.!?])\s+/)
    .filter((part) => part.trim() !== '').length;
}

export function summarize(text, sentences = 2) {
  const parts = singleLine(text).split(/(?<=[.!?])\s+/);
  return parts.slice(0, sentences).join(' ').trim();
}

export function indent(text, width = 2) {
  const pad = ' '.repeat(Math.max(0, width));
  return String(text ?? '')
    .split('\n')
    .map((line) => (line === '' ? line : `${pad}${line}`))
    .join('\n');
}

export function ordinal(value) {
  const number = Math.trunc(Number(value) || 0);
  const rest = number % 100;
  if (rest >= 11 && rest <= 13) return `${number}th`;
  switch (number % 10) {
    case 1:
      return `${number}st`;
    case 2:
      return `${number}nd`;
    case 3:
      return `${number}rd`;
    default:
      return `${number}th`;
  }
}

export function wordFrequency(text, limit = 10) {
  const counts = new Map();
  for (const word of tokenWords(text)) counts.set(word, (counts.get(word) ?? 0) + 1);
  return [...counts.entries()]
    .map(([word, count]) => ({ word, count }))
    .sort((left, right) => right.count - left.count || (left.word < right.word ? -1 : 1))
    .slice(0, limit);
}

export function tokenWords(text) {
  return singleLine(text)
    .toLowerCase()
    .split(/[^a-z0-9']+/)
    .filter((word) => word.length > 2);
}

export function escapeCsvText(value) {
  const text = String(value ?? '');
  return /[",\n]/.test(text) ? `"${text.replace(/"/g, '""')}"` : text;
}

export function bytesLabel(bytes) {
  const size = Number(bytes) || 0;
  if (size < 1024) return `${size} B`;
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KiB`;
  return `${(size / (1024 * 1024)).toFixed(1)} MiB`;
}
