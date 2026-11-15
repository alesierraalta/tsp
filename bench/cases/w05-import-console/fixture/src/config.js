export const LIMITS = {
  maxRows: 5000,
  maxColumns: 60,
  maxCellLength: 2000,
  maxPage: 100,
  maxFileBytes: 8 * 1024 * 1024,
};

export const DEFAULTS = {
  title: 'Import console',
  locale: 'en',
  currency: 'USD',
  pageSize: 25,
  dateFormat: 'iso',
  strictUnknownColumns: false,
  moneyColumns: ['amount', 'total', 'subtotal', 'balance'],
};

const KNOWN_KEYS = new Set(Object.keys(DEFAULTS));

export function loadConfig(overrides = {}) {
  if (overrides === null || typeof overrides !== 'object') {
    throw new TypeError('configuration overrides must be an object');
  }
  for (const key of Object.keys(overrides)) {
    if (!KNOWN_KEYS.has(key)) {
      throw new TypeError(`unknown configuration key: ${key}`);
    }
  }
  const config = { ...DEFAULTS, ...overrides };
  if (typeof config.title !== 'string' || config.title.length === 0) {
    throw new TypeError('the console title must be a non empty string');
  }
  if (!Number.isInteger(config.pageSize) || config.pageSize < 1 || config.pageSize > LIMITS.maxPage) {
    throw new TypeError(`pageSize must be between 1 and ${LIMITS.maxPage}`);
  }
  if (!['USD', 'EUR', 'GBP', 'ARS'].includes(config.currency)) {
    throw new TypeError(`unsupported currency: ${config.currency}`);
  }
  if (!['iso', 'short', 'long'].includes(config.dateFormat)) {
    throw new TypeError(`unsupported date format: ${config.dateFormat}`);
  }
  if (!Array.isArray(config.moneyColumns) || config.moneyColumns.some((name) => typeof name !== 'string')) {
    throw new TypeError('moneyColumns must be a list of column names');
  }
  return config;
}

export function datasetLimit(config, requested) {
  const wanted = Number(requested ?? config.pageSize);
  if (!Number.isFinite(wanted) || wanted <= 0) return config.pageSize;
  return Math.min(Math.floor(wanted), LIMITS.maxPage);
}

export function rowBudget(config) {
  return Math.min(LIMITS.maxRows, Number(config.maxRows ?? LIMITS.maxRows));
}

export function describeConfig(config) {
  return [
    `title=${config.title}`,
    `currency=${config.currency}`,
    `pageSize=${config.pageSize}`,
    `dateFormat=${config.dateFormat}`,
    `moneyColumns=${config.moneyColumns.join('+')}`,
  ].join(' ');
}

export function withOverrides(config, overrides) {
  return loadConfig({ ...config, ...overrides });
}

export function isMoneyColumn(config, name) {
  return config.moneyColumns.includes(String(name).toLowerCase());
}
