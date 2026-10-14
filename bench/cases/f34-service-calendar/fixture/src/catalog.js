export class Catalog {
  #services = new Map();

  register(id, options = {}) {
    if (typeof id !== 'string' || !/^[a-z0-9-]+$/.test(id)) throw new TypeError('id');
    if (this.#services.has(id)) throw new Error('service exists');
    const capacity = options.capacity ?? 10;
    const duration = options.duration ?? 3600000;
    if (!Number.isInteger(capacity) || capacity < 1) throw new TypeError('capacity');
    if (!Number.isSafeInteger(duration) || duration < 1) throw new TypeError('duration');
    const service = {
      id,
      capacity,
      duration,
      enabled: true,
      tags: [...new Set(options.tags ?? [])].sort(),
    };
    this.#services.set(id, service);
    return structuredClone(service);
  }

  get(id) {
    const service = this.#services.get(id);
    if (!service) throw new Error('unknown service');
    return structuredClone(service);
  }

  setEnabled(id, enabled) {
    if (typeof enabled !== 'boolean') throw new TypeError('enabled');
    this.get(id);
    this.#services.get(id).enabled = enabled;
  }

  list(tag) {
    return [...this.#services.values()]
      .filter(service => tag === undefined || service.tags.includes(tag))
      .sort((a, b) => a.id.localeCompare(b.id))
      .map(service => structuredClone(service));
  }

  requireEnabled(id) {
    const service = this.get(id);
    if (!service.enabled) throw new Error('service disabled');
    return service;
  }

  get size() {
    return this.#services.size;
  }
}
