const LEVELS = {
  debug: 10,
  info: 20,
  warn: 30,
  error: 40,
  silent: 99,
};

function serialize(entry) {
  const parts = [entry.time, entry.level.toUpperCase(), entry.message];
  for (const [key, value] of Object.entries(entry.fields)) {
    if (value === undefined) continue;
    parts.push(`${key}=${typeof value === 'object' ? JSON.stringify(value) : String(value)}`);
  }
  return parts.join(' ');
}

export function createLogger(options = {}) {
  const level = options.level ?? 'info';
  if (!(level in LEVELS)) {
    throw new TypeError(`unknown log level: ${level}`);
  }
  const sink = options.sink ?? ((line) => process.stderr.write(`${line}\n`));
  const fields = { ...(options.fields ?? {}) };
  const clock = options.clock ?? (() => new Date());
  const entries = options.entries ?? [];

  function emit(name, message, extra = {}) {
    if (LEVELS[name] < LEVELS[level]) return undefined;
    const entry = {
      time: new Date(clock()).toISOString(),
      level: name,
      message: String(message),
      fields: { ...fields, ...extra },
    };
    entries.push(entry);
    sink(serialize(entry));
    return entry;
  }

  return {
    level,
    entries,
    debug: (message, extra) => emit('debug', message, extra),
    info: (message, extra) => emit('info', message, extra),
    warn: (message, extra) => emit('warn', message, extra),
    error: (message, extra) => emit('error', message, extra),
    child(extra) {
      return createLogger({ level, sink, fields: { ...fields, ...extra }, clock, entries });
    },
    recent(count = 20) {
      return entries.slice(-count);
    },
  };
}
