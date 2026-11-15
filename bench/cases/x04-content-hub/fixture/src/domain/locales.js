export const DEFAULT_LOCALES = [
  { code: 'en', name: 'English', default: true },
  { code: 'es', name: 'Spanish' },
  { code: 'de', name: 'German' },
  { code: 'fr', name: 'French' },
];

const PATTERN = /^[a-z]{2}(-[A-Z]{2})?$/;

export function isLocaleCode(code) {
  return PATTERN.test(String(code ?? ''));
}

export function normalizeLocale(input) {
  const text = String(input ?? '').trim();
  if (text === '') return undefined;
  const [language, region] = text.replace('_', '-').split('-');
  const normalized = region
    ? `${language.toLowerCase()}-${region.toUpperCase()}`
    : language.toLowerCase();
  return isLocaleCode(normalized) ? normalized : undefined;
}

export class LocaleSet {
  constructor(list = DEFAULT_LOCALES) {
    this.locales = list.map((locale) => ({ ...locale }));
    if (!this.locales.some((locale) => locale.default)) this.locales[0].default = true;
  }

  codes() {
    return this.locales.map((locale) => locale.code);
  }

  has(code) {
    const normalized = normalizeLocale(code);
    return normalized !== undefined && this.locales.some((locale) => locale.code === normalized);
  }

  default() {
    return this.locales.find((locale) => locale.default)?.code ?? 'en';
  }

  add(input) {
    const code = normalizeLocale(input?.code);
    if (!code) throw new TypeError('a locale needs a valid code');
    if (this.has(code)) throw new TypeError(`locale ${code} already exists`);
    const locale = { code, name: String(input?.name ?? code), default: false };
    this.locales.push(locale);
    return locale;
  }

  remove(code) {
    const normalized = normalizeLocale(code);
    const index = this.locales.findIndex((locale) => locale.code === normalized);
    if (index < 0) return false;
    if (this.locales[index].default) throw new TypeError('the default locale cannot be removed');
    this.locales.splice(index, 1);
    return true;
  }

  fallbackChain(code) {
    const normalized = normalizeLocale(code) ?? this.default();
    const [language] = normalized.split('-');
    const chain = [normalized];
    if (language !== normalized && this.has(language)) chain.push(language);
    if (!chain.includes(this.default())) chain.push(this.default());
    return chain;
  }

  resolve(records, locale) {
    for (const candidate of this.fallbackChain(locale)) {
      const found = records.find((record) => record.locale === candidate);
      if (found) return found;
    }
    return undefined;
  }
}
