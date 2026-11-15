export const LEVELS = ['debug', 'info', 'warn', 'error', 'silent'];

export function levelOf(name) {
  const index = LEVELS.indexOf(String(name ?? '').toLowerCase());
  return index < 0 ? LEVELS.indexOf('info') : index;
}

export function shouldLog(level, configured) {
  return levelOf(level) >= levelOf(configured);
}

export function createLogger(options = {}) {
  const configured = options.level ?? 'info';
  const sink = options.sink ?? ((line) => process.stdout.write(`${line}\n`));
  const base = { service: options.service ?? 'release-dashboard' };

  const emit = (level, message, fields = {}) => {
    if (!shouldLog(level, configured)) return;
    const record = { level, message, at: new Date(options.now?.() ?? Date.now()).toISOString(), ...base, ...fields };
    sink(JSON.stringify(record));
  };

  return {
    level: configured,
    debug: (message, fields) => emit('debug', message, fields),
    info: (message, fields) => emit('info', message, fields),
    warn: (message, fields) => emit('warn', message, fields),
    error: (message, fields) => emit('error', message, fields),
    child: (fields) => createLogger({ ...options, ...fields }),
  };
}

export function silentLogger() {
  return createLogger({ level: 'silent', sink: () => {} });
}

export function memorySink() {
  const lines = [];
  return {
    lines,
    sink: (line) => lines.push(line),
    records: () => lines.map((line) => JSON.parse(line)),
  };
}

export function describeLogger(logger) {
  return { level: logger.level };
}

export function redactSecrets(fields = {}) {
  const out = {};
  for (const [name, value] of Object.entries(fields)) {
    if (/secret|token|password|authorization/i.test(name)) {
      out[name] = '***';
      continue;
    }
    out[name] = value;
  }
  return out;
}
