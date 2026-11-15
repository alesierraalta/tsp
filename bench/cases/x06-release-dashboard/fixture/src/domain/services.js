import { badRequest } from '../util/errors.js';
import { slugify, singleLine } from '../util/text.js';
import { nowIso } from '../util/time.js';

export const LANGUAGES = ['node', 'go', 'python', 'rust'];

export function normalizeService(input = {}, options = {}) {
  const name = singleLine(input.name ?? '');
  if (name === '') throw badRequest('invalid_service', 'a service needs a name');
  const language = LANGUAGES.includes(input.language) ? input.language : 'node';
  return {
    id: slugify(name) || 'service',
    name,
    language,
    owner: singleLine(input.owner ?? 'platform'),
    repo: singleLine(input.repo ?? `example/${slugify(name)}`),
    created_at: input.created_at ?? nowIso(options.clock),
  };
}

export class ServiceCatalog {
  constructor(options = {}) {
    this.clock = options.clock ?? (() => new Date());
    this.services = new Map();
  }

  define(input) {
    const service = normalizeService(input, { clock: this.clock });
    if (this.services.has(service.id)) throw badRequest('service_exists', `service ${service.id} already exists`);
    this.services.set(service.id, service);
    return service;
  }

  get(id) {
    return this.services.get(String(id));
  }

  require(id) {
    const service = this.get(id);
    if (!service) throw badRequest('service_missing', `service ${id} is not registered`);
    return service;
  }

  update(id, patch) {
    const service = this.require(id);
    const next = { ...service, ...patch, id: service.id, name: service.name };
    this.services.set(service.id, next);
    return next;
  }

  list(language) {
    const all = [...this.services.values()];
    return (language === undefined ? all : all.filter((service) => service.language === language)).sort((left, right) =>
      left.name < right.name ? -1 : 1,
    );
  }

  remove(id) {
    return this.services.delete(String(id));
  }

  byOwner() {
    const out = {};
    for (const service of this.services.values()) out[service.owner] = (out[service.owner] ?? 0) + 1;
    return out;
  }

  stats() {
    const languages = {};
    for (const service of this.services.values()) languages[service.language] = (languages[service.language] ?? 0) + 1;
    return { services: this.services.size, languages };
  }
}

export function serviceRows(services) {
  return services.map((service) => ({ id: service.id, name: service.name, owner: service.owner, language: service.language }));
}

export function ownerOf(services, id) {
  return services.get(String(id))?.owner;
}

export function catalogReport(catalog) {
  const stats = catalog.stats();
  return {
    services: stats.services,
    languages: stats.languages,
    owners: Object.keys(catalog.byOwner()).length,
    byOwner: catalog.byOwner(),
  };
}

export function servicesForOwner(catalog, owner) {
  return catalog.list().filter((service) => service.owner === owner).map((service) => service.id);
}

export function missingOwners(catalog) {
  return catalog.list().filter((service) => service.owner === '' || service.owner === 'platform');
}
