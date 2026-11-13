const LEVELS = { debug: 10, info: 20, warn: 30, error: 40, silent: 100 };

const SECRETS = ['authorization', 'apikey', 'api_key', 'secret', 'token', 'signature'];

export function redact(value) {
  if (value === null || typeof value !== 'object') return value;
  if (Array.isArray(value)) return value.map((item) => redact(item));
  const out = {};
  for (const [key, item] of Object.entries(value)) {
    out[key] = SECRETS.includes(key.toLowerCase()) ? '[redacted]' : redact(item);
  }
  return out;
}

export function createLogger(options = {}) {
  const level = options.level ?? 'info';
  const threshold = LEVELS[level] ?? LEVELS.info;
  const sink = options.sink ?? ((line) => process.stdout.write(`${line}\n`));
  const clock = options.clock ?? (() => new Date());
  const base = options.base ?? {};
  const entries = [];

  const emit = (name, message, fields) => {
    const entry = { at: new Date(clock()).toISOString(), level: name, message, ...redact({ ...base, ...(fields ?? {}) }) };
    entries.push(entry);
    if (LEVELS[name] >= threshold) sink(JSON.stringify(entry));
    return entry;
  };

  return {
    level,
    entries,
    child(fields) {
      return createLogger({ level, sink, base: { ...base, ...fields }, clock });
    },
    debug: (message, fields) => emit('debug', message, fields),
    info: (message, fields) => emit('info', message, fields),
    warn: (message, fields) => emit('warn', message, fields),
    error: (message, fields) => emit('error', message, fields),
    clear() {
      entries.length = 0;
    },
  };
}

export function timed(logger, label, run) {
  const started = Date.now();
  const result = run();
  logger.debug(label, { ms: Date.now() - started });
  return result;
}
