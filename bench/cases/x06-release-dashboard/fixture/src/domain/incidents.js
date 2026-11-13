import { badRequest, conflict } from '../util/errors.js';
import { newId } from '../util/ids.js';
import { nowIso, diffMinutes } from '../util/time.js';
import { singleLine } from '../util/text.js';

export const SEVERITIES = ['sev1', 'sev2', 'sev3', 'sev4'];

export const SEVERITY_RANK = { sev1: 4, sev2: 3, sev3: 2, sev4: 1 };

export function severityOf(value, fallback = 'sev3') {
  return SEVERITIES.includes(String(value)) ? String(value) : fallback;
}

export function severityRank(value) {
  return SEVERITY_RANK[severityOf(value)] ?? 0;
}

export function isBlocking(value) {
  return severityRank(value) >= SEVERITY_RANK.sev2;
}

export function normalizeIncident(input = {}, options = {}) {
  const title = singleLine(input.title ?? '');
  if (title === '') throw badRequest('invalid_incident', 'an incident needs a title');
  return {
    id: input.id ?? newId('inc'),
    tenant: String(input.tenant ?? options.tenant ?? 'public'),
    title,
    severity: severityOf(input.severity),
    service: String(input.service ?? ''),
    environment: String(input.environment ?? 'production'),
    deployment: input.deployment ?? null,
    state: 'open',
    opened_by: String(input.opened_by ?? options.actor ?? 'system'),
    owner: input.owner === undefined ? null : String(input.owner),
    opened_at: nowIso(options.clock),
    resolved_at: null,
    resolution: null,
  };
}

export class IncidentBook {
  constructor(options = {}) {
    this.clock = options.clock ?? (() => new Date());
    this.incidents = new Map();
  }

  open(input) {
    const incident = normalizeIncident(input, { clock: this.clock });
    this.incidents.set(incident.id, incident);
    return incident;
  }

  get(id) {
    return this.incidents.get(String(id));
  }

  require(id) {
    const incident = this.get(id);
    if (!incident) throw badRequest('incident_missing', `incident ${id} does not exist`);
    return incident;
  }

  assign(id, owner) {
    const incident = this.require(id);
    const next = { ...incident, owner: String(owner) };
    this.incidents.set(id, next);
    return next;
  }

  resolve(id, options = {}) {
    const incident = this.require(id);
    if (incident.state !== 'open') throw conflict('incident_closed', `incident ${id} is ${incident.state}`);
    const next = {
      ...incident,
      state: 'resolved',
      resolved_at: nowIso(this.clock),
      resolution: String(options.resolution ?? 'resolved'),
    };
    this.incidents.set(id, next);
    return next;
  }

  list(options = {}) {
    return [...this.incidents.values()]
      .filter((incident) => options.tenant === undefined || incident.tenant === options.tenant)
      .filter((incident) => options.state === undefined || incident.state === options.state)
      .filter((incident) => options.service === undefined || incident.service === options.service)
      .sort((left, right) => severityRank(right.severity) - severityRank(left.severity) || (left.opened_at < right.opened_at ? 1 : -1));
  }

  blocking() {
    return this.list({ state: 'open' }).filter((incident) => isBlocking(incident.severity));
  }

  durationMinutes(incident, clock = () => new Date()) {
    return diffMinutes(incident.opened_at, incident.resolved_at ?? new Date(clock()));
  }

  summary(clock = () => new Date()) {
    const open = this.list({ state: 'open' });
    return {
      incidents: this.incidents.size,
      open: open.length,
      blocking: open.filter((incident) => isBlocking(incident.severity)).length,
      by_severity: open.reduce((out, incident) => ({ ...out, [incident.severity]: (out[incident.severity] ?? 0) + 1 }), {}),
      mean_minutes: open.length === 0 ? 0 : Math.round(open.reduce((total, incident) => total + this.durationMinutes(incident, clock), 0) / open.length),
    };
  }

  stats() {
    return this.summary();
  }
}

export function incidentsFor(incidents, service) {
  return (incidents ?? []).filter((incident) => incident.service === service);
}

export function openBySeverity(incidents) {
  const out = { sev1: 0, sev2: 0, sev3: 0, sev4: 0 };
  for (const incident of incidents ?? []) {
    if (incident.state !== 'open') continue;
    out[incident.severity] = (out[incident.severity] ?? 0) + 1;
  }
  return out;
}

export function oldestOpen(incidents) {
  return (incidents ?? [])
    .filter((incident) => incident.state === 'open')
    .sort((left, right) => (left.opened_at < right.opened_at ? -1 : 1))[0];
}

export function resolutionRate(incidents) {
  const total = (incidents ?? []).length;
  const resolved = (incidents ?? []).filter((incident) => incident.state === 'resolved').length;
  return { total, resolved, rate: total === 0 ? 0 : Math.round((resolved / total) * 10000) / 100 };
}

export function escalateTo(incident, severity) {
  const next = severityOf(severity);
  if (severityRank(next) <= severityRank(incident.severity)) return { ...incident };
  return { ...incident, severity: next };
}
