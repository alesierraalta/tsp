import { conflict, notFound } from '../util/errors.js';

export const EVENTS = ['import.finished', 'import.failed', 'delivery.failed', 'delivery.delivered'];

export class HookRegistry {
  constructor(hooks = []) {
    this.hooks = new Map();
    for (const hook of hooks) this.add(hook);
  }

  add(hook) {
    if (!hook || typeof hook !== 'object') throw new TypeError('a hook object is required');
    const name = String(hook.name ?? '').trim();
    if (name.length === 0) throw new TypeError('a hook needs a name');
    if (!EVENTS.includes(hook.event)) throw new TypeError(`unknown hook event: ${hook.event}`);
    if (this.hooks.has(name)) throw conflict(`hook ${name} already exists`);
    this.hooks.set(name, {
      name,
      event: hook.event,
      script: String(hook.script ?? ''),
      enabled: hook.enabled !== false,
      timeoutMs: hook.timeoutMs,
    });
    return this.hooks.get(name);
  }

  get(name) {
    const hook = this.hooks.get(name);
    if (!hook) throw notFound(`hook ${name}`);
    return hook;
  }

  forEvent(event) {
    return [...this.hooks.values()].find((hook) => hook.event === event && hook.enabled);
  }

  allForEvent(event) {
    return [...this.hooks.values()].filter((hook) => hook.event === event && hook.enabled);
  }

  disable(name) {
    const hook = this.get(name);
    hook.enabled = false;
    return hook;
  }

  enable(name) {
    const hook = this.get(name);
    hook.enabled = true;
    return hook;
  }

  remove(name) {
    const hook = this.get(name);
    this.hooks.delete(name);
    return hook;
  }

  list() {
    return [...this.hooks.values()].sort((left, right) => (left.name < right.name ? -1 : 1));
  }

  count() {
    return this.hooks.size;
  }

  describe() {
    return this.list().map((hook) => `${hook.name}:${hook.event}:${hook.enabled ? 'on' : 'off'}`);
  }
}
