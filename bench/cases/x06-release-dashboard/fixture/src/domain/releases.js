import { badRequest, conflict } from '../util/errors.js';
import { parseVersion, isVersion, channelOf } from './versions.js';
import { environmentOf, isProtected } from './environments.js';
import { newId } from '../util/ids.js';
import { nowIso } from '../util/time.js';
import { singleLine } from '../util/text.js';
import { normalizeChanges } from './changes.js';

export const RELEASE_STATUSES = ['planned', 'staged', 'canary', 'live', 'rolled_back', 'cancelled'];

export const TRANSITIONS = {
  planned: ['staged', 'cancelled'],
  staged: ['canary', 'cancelled'],
  canary: ['live', 'rolled_back', 'cancelled'],
  live: ['rolled_back'],
  rolled_back: [],
  cancelled: [],
};

export function canTransition(from, to) {
  return (TRANSITIONS[from] ?? []).includes(to);
}

export function normalizeRelease(input = {}, options = {}) {
  const version = singleLine(input.version ?? '');
  if (!isVersion(version)) throw badRequest('invalid_version', `${input.version} is not a semantic version`);
  const service = singleLine(input.service ?? '');
  if (service === '') throw badRequest('invalid_service', 'a release names the service it ships');
  const changelog = Array.isArray(input.changelog) ? normalizeChanges(input.changelog) : [];
  return {
    id: input.id ?? newId('rel'),
    tenant: String(input.tenant ?? options.tenant ?? 'public'),
    version,
    channel: channelOf(version),
    major: parseVersion(version).major,
    service,
    environment: environmentOf(input.environment),
    status: 'planned',
    changelog,
    summary: singleLine(input.summary ?? changelog[0]?.summary ?? `${service} ${version}`),
    created_by: String(input.created_by ?? options.actor ?? 'system'),
    created_at: input.created_at ?? nowIso(options.clock),
    updated_at: null,
  };
}

export function describeRelease(release) {
  return {
    id: release.id,
    version: release.version,
    channel: release.channel,
    service: release.service,
    environment: release.environment,
    status: release.status,
  };
}

export class ReleaseBook {
  constructor(options = {}) {
    this.clock = options.clock ?? (() => new Date());
    this.events = options.events;
    this.releases = new Map();
  }

  create(input) {
    const release = normalizeRelease(input, { clock: this.clock });
    if (this.releases.has(release.id)) throw conflict('release_exists', `release ${release.id} exists`);
    const duplicate = this.find(release.service, release.version, release.environment);
    if (duplicate) throw conflict('release_duplicate', `${release.service} ${release.version} already exists for ${release.environment}`);
    this.releases.set(release.id, release);
    this.events?.append('release.created', { version: release.version, service: release.service }, { tenant: release.tenant, actor: release.created_by, subject: release.id });
    return release;
  }

  get(id) {
    return this.releases.get(String(id));
  }

  require(id) {
    const release = this.get(id);
    if (!release) throw badRequest('release_missing', `release ${id} does not exist`);
    return release;
  }

  find(service, version, environment) {
    return [...this.releases.values()].find(
      (release) => release.service === service && release.version === version && release.environment === environmentOf(environment),
    );
  }

  list(options = {}) {
    return [...this.releases.values()]
      .filter((release) => options.tenant === undefined || release.tenant === options.tenant)
      .filter((release) => options.service === undefined || release.service === options.service)
      .filter((release) => options.environment === undefined || release.environment === options.environment)
      .filter((release) => options.status === undefined || release.status === options.status)
      .filter((release) => options.channel === undefined || release.channel === options.channel)
      .sort((left, right) => (left.created_at < right.created_at ? 1 : -1));
  }

  transition(id, status, options = {}) {
    const release = this.require(id);
    if (!RELEASE_STATUSES.includes(status)) throw badRequest('invalid_status', `${status} is not a release status`);
    if (!canTransition(release.status, status)) {
      throw conflict('invalid_transition', `${release.service} ${release.version} cannot go from ${release.status} to ${status}`);
    }
    const next = { ...release, status, updated_at: nowIso(this.clock) };
    this.releases.set(id, next);
    this.events?.append('release.created', { status }, { tenant: next.tenant, actor: options.actor, subject: next.id });
    return next;
  }

  promote(id, options = {}) {
    const release = this.require(id);
    if (release.environment === 'production') throw conflict('invalid_transition', 'production is the last environment');
    const next = release.environment === 'development' ? 'staging' : 'production';
    const promoted = { ...release, environment: next, status: 'planned', updated_at: nowIso(this.clock) };
    this.releases.set(id, promoted);
    this.events?.append('release.created', { promoted: next }, { tenant: promoted.tenant, actor: options.actor, subject: promoted.id });
    return promoted;
  }

  cancel(id, options = {}) {
    return this.transition(id, 'cancelled', options);
  }

  liveFor(environment) {
    return this.list({ environment, status: 'live' });
  }

  protectedOnes() {
    return this.list().filter((release) => isProtected(release.environment));
  }

  remove(id) {
    return this.releases.delete(String(id));
  }

  stats() {
    const byStatus = {};
    const byEnvironment = {};
    for (const release of this.releases.values()) {
      byStatus[release.status] = (byStatus[release.status] ?? 0) + 1;
      byEnvironment[release.environment] = (byEnvironment[release.environment] ?? 0) + 1;
    }
    return { releases: this.releases.size, byStatus, byEnvironment };
  }
}

export function releaseWindows(releases) {
  const byService = new Map();
  for (const release of releases ?? []) {
    const bucket = byService.get(release.service) ?? [];
    bucket.push(release);
    byService.set(release.service, bucket);
  }
  return [...byService.entries()].map(([service, entries]) => {
    const sorted = [...entries].sort((left, right) => (left.created_at < right.created_at ? -1 : 1));
    return {
      service,
      releases: sorted.length,
      first: sorted[0]?.created_at,
      last: sorted[sorted.length - 1]?.created_at,
      live: sorted.filter((release) => release.status === 'live').length,
    };
  });
}

export function stalePlanned(releases, days, clock = () => new Date()) {
  const horizon = new Date(clock()).getTime() - Math.max(0, Math.trunc(days)) * 86400000;
  return (releases ?? []).filter(
    (release) => release.status === 'planned' && new Date(release.created_at).getTime() < horizon,
  );
}

export function channelBreakdown(releases) {
  const out = {};
  for (const release of releases ?? []) out[release.channel] = (out[release.channel] ?? 0) + 1;
  return out;
}

export function blockingReleases(releases, incidents) {
  const blockingServices = new Set(
    (incidents ?? []).filter((incident) => incident.state === 'open' && ['sev1', 'sev2'].includes(incident.severity)).map((incident) => incident.service),
  );
  return (releases ?? []).filter((release) => blockingServices.has(release.service) && release.environment === 'production');
}

export function releaseStatusLine(release) {
  return `${release.service} ${release.version} is ${release.status} on ${release.environment}`;
}

export function summarizeReleases(releases) {
  const list = releases ?? [];
  return {
    releases: list.length,
    planned: list.filter((release) => release.status === 'planned').length,
    live: list.filter((release) => release.status === 'live').length,
    rolledBack: list.filter((release) => release.status === 'rolled_back').length,
    channels: channelBreakdown(list),
    services: new Set(list.map((release) => release.service)).size,
  };
}
