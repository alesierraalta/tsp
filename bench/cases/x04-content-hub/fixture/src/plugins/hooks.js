import { createLogger } from '../util/log.js';

export class HookBus {
  constructor(options = {}) {
    this.registry = options.registry;
    this.logger = options.logger ?? createLogger({ level: 'silent' });
    this.filters = new Map();
    this.calls = [];
  }

  on(hook, fn) {
    const bucket = this.filters.get(hook);
    if (bucket) bucket.push(fn);
    else this.filters.set(hook, [fn]);
    return fn;
  }

  listeners(hook) {
    return [...(this.filters.get(hook) ?? [])];
  }

  async emit(hook, payload) {
    const started = Date.now();
    let value = payload;
    const manifests = this.registry ? this.registry.forHook(hook) : [];
    for (const manifest of manifests) {
      value = { ...value, [`plugin_${manifest.name}`]: true };
    }
    for (const fn of this.listeners(hook)) {
      const next = await fn(value);
      if (next !== undefined) value = next;
    }
    this.calls.push({ hook, plugins: manifests.length, ms: Date.now() - started });
    this.logger.debug('hook', { hook, plugins: manifests.length });
    return value;
  }

  async emitAll(hooks, payload) {
    let value = payload;
    for (const hook of hooks) value = await this.emit(hook, value);
    return value;
  }

  history(limit = 20) {
    return this.calls.slice(-limit);
  }

  clear() {
    this.filters.clear();
    this.calls.length = 0;
  }

  stats() {
    const out = {};
    for (const call of this.calls) out[call.hook] = (out[call.hook] ?? 0) + 1;
    return out;
  }

  once(hook, fn) {
    const wrapped = async (payload) => {
      this.filters.set(hook, this.listeners(hook).filter((listener) => listener !== wrapped));
      return fn(payload);
    };
    this.on(hook, wrapped);
    return wrapped;
  }

  off(hook) {
    return this.filters.delete(hook);
  }

  hookNames() {
    return [...this.filters.keys()].sort();
  }

  listenerCount(hook) {
    return this.listeners(hook).length;
  }
}

export function assertHookName(name, allowed) {
  if (!allowed.includes(name)) throw new TypeError(`unknown hook ${name}`);
  return name;
}
