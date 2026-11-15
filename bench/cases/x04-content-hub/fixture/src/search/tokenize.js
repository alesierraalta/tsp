const STOPWORDS = new Set([
  'the', 'and', 'for', 'with', 'that', 'this', 'from', 'into', 'your', 'have', 'will', 'are',
  'was', 'were', 'not', 'but', 'you', 'our', 'their', 'its', 'about', 'over', 'under',
]);

export function fold(text) {
  return String(text ?? '')
    .normalize('NFKD')
    .replace(/[\u0300-\u036f]/g, '')
    .toLowerCase();
}

export function tokenize(text, options = {}) {
  const folded = fold(text);
  const parts = folded.split(/[^a-z0-9]+/).filter((part) => part.length >= (options.minLength ?? 2));
  if (options.stopwords === false) return parts;
  return parts.filter((part) => !STOPWORDS.has(part));
}

export function stem(token) {
  const text = String(token);
  if (text.length > 4 && text.endsWith('ies')) return `${text.slice(0, -3)}y`;
  if (text.length > 4 && /(?:x|z|ch|sh)es$/.test(text)) return text.slice(0, -2);
  if (text.length > 4 && text.endsWith('ses')) return text.slice(0, -2);
  if (text.length > 3 && text.endsWith('s') && !text.endsWith('ss')) return text.slice(0, -1);
  return text;
}

export function tokens(text, options = {}) {
  return tokenize(text, options).map((token) => stem(token));
}

export function termFrequency(text) {
  const out = new Map();
  for (const token of tokens(text)) out.set(token, (out.get(token) ?? 0) + 1);
  return out;
}

export function snippet(text, needle, radius = 60) {
  const source = String(text ?? '');
  const folded = fold(source);
  const at = folded.indexOf(fold(needle));
  if (at < 0) return source.slice(0, radius * 2);
  const start = Math.max(0, at - radius);
  const end = Math.min(source.length, at + String(needle).length + radius);
  return `${start > 0 ? '…' : ''}${source.slice(start, end)}${end < source.length ? '…' : ''}`;
}

export function phrases(text, size = 2) {
  const parts = tokenize(text);
  const out = [];
  for (let index = 0; index + size <= parts.length; index += 1) {
    out.push(parts.slice(index, index + size).join(' '));
  }
  return out;
}

export function queryTerms(query) {
  return tokens(query).filter((token, index, all) => all.indexOf(token) === index);
}

export function containsTerm(text, term) {
  return tokens(text).includes(stem(String(term)));
}

export function scoreText(text, terms) {
  const own = tokens(text);
  let score = 0;
  for (const term of terms) {
    if (own.includes(term)) score += 1;
  }
  return score;
}

export function vocabularySize(texts) {
  const set = new Set();
  for (const text of texts ?? []) for (const token of tokens(text)) set.add(token);
  return set.size;
}
