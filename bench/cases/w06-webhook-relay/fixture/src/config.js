import { parseWindow } from './queue/window.js';

export const DEFAULTS = {
  concurrency: 4,
  maxAttempts: 5,
  timeoutMs: 5000,
  batchSize: 50,
  allowInsecure: false,
  allowInternalHosts: false,
  allowlist: [],
  hookTimeoutMs: 10000,
  deliveryWindow: '00:00-23:59',
  quietHours: [],
  keyMaxAgeDays: 30,
};

const KNOWN = new Set(Object.keys(DEFAULTS));

export function loadConfig(overrides = {}) {
  if (overrides === null || typeof overrides !== 'object') {
    throw new TypeError('configuration overrides must be an object');
  }
  for (const key of Object.keys(overrides)) {
    if (!KNOWN.has(key)) throw new TypeError(`unknown configuration key: ${key}`);
  }
  const config = { ...DEFAULTS, ...overrides };
  if (!Number.isInteger(config.concurrency) || config.concurrency < 1 || config.concurrency > 32) {
    throw new TypeError('concurrency must be a whole number between 1 and 32');
  }
  if (!Number.isInteger(config.maxAttempts) || config.maxAttempts < 1 || config.maxAttempts > 20) {
    throw new TypeError('maxAttempts must be a whole number between 1 and 20');
  }
  if (!Number.isInteger(config.timeoutMs) || config.timeoutMs < 100) {
    throw new TypeError('timeoutMs must be at least 100');
  }
  if (!Array.isArray(config.allowlist) || config.allowlist.some((host) => typeof host !== 'string')) {
    throw new TypeError('allowlist must be a list of hostnames');
  }
  parseWindow(config.deliveryWindow);
  if (!Array.isArray(config.quietHours)) {
    throw new TypeError('quietHours must be a list of windows');
  }
  return config;
}

export function configFromEnv(env = process.env) {
  const overrides = {};
  if (env.RELAY_CONCURRENCY) overrides.concurrency = Number(env.RELAY_CONCURRENCY);
  if (env.RELAY_MAX_ATTEMPTS) overrides.maxAttempts = Number(env.RELAY_MAX_ATTEMPTS);
  if (env.RELAY_TIMEOUT_MS) overrides.timeoutMs = Number(env.RELAY_TIMEOUT_MS);
  if (env.RELAY_BATCH_SIZE) overrides.batchSize = Number(env.RELAY_BATCH_SIZE);
  if (env.RELAY_ALLOW_INSECURE) overrides.allowInsecure = env.RELAY_ALLOW_INSECURE === '1';
  if (env.RELAY_ALLOW_INTERNAL) overrides.allowInternalHosts = env.RELAY_ALLOW_INTERNAL === '1';
  if (env.RELAY_ALLOWLIST) {
    overrides.allowlist = env.RELAY_ALLOWLIST.split(',')
      .map((host) => host.trim())
      .filter(Boolean);
  }
  return loadConfig(overrides);
}

export function describeConfig(config) {
  return [
    `concurrency=${config.concurrency}`,
    `maxAttempts=${config.maxAttempts}`,
    `timeoutMs=${config.timeoutMs}`,
    `allowInsecure=${config.allowInsecure}`,
    `allowlist=${config.allowlist.length}`,
    `window=${config.deliveryWindow}`,
  ].join(' ');
}

export function retryDelay(attempt, config) {
  const base = Math.min(30_000, 250 * 2 ** Math.max(0, attempt - 1));
  return Math.min(base, config.timeoutMs * 2);
}

export function shouldRetry(error, attempt, config) {
  if (attempt >= config.maxAttempts) return false;
  if (error && error.retryable === false) return false;
  return true;
}

export function mergeConfig(base, overrides) {
  return loadConfig({ ...base, ...overrides });
}

export function windowSeconds(config) {
  return parseWindow(config.deliveryWindow).minutes * 60;
}

export function isRetryable(error) {
  return Boolean(error) && error.retryable !== false;
}

export function describeLimits(config) {
  return `rows=${config.batchSize} attempts=${config.maxAttempts} window=${config.deliveryWindow}`;
}
