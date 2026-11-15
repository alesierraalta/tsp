import { sortBy } from '../util/collections.js';
import { diffMinutes, sortableInstant } from '../util/time.js';

export const TIMELINE_KINDS = ['release', 'deployment', 'rollback', 'incident', 'approval'];

export function timelineEntry(kind, at, data = {}) {
  if (!TIMELINE_KINDS.includes(kind)) throw new TypeError(`unknown timeline kind ${kind}`);
  return { kind, at, ...data };
}

export function releaseEntries(releases) {
  return (releases ?? []).map((release) =>
    timelineEntry('release', release.created_at, {
      subject: release.id,
      label: `${release.service} ${release.version}`,
      service: release.service,
      environment: release.environment,
      status: release.status,
    }),
  );
}

export function deploymentEntries(deployments) {
  const out = [];
  for (const deployment of deployments ?? []) {
    out.push(
      timelineEntry('deployment', deployment.started_at, {
        subject: deployment.id,
        label: `${deployment.service} ${deployment.version}`,
        service: deployment.service,
        environment: deployment.environment,
        status: deployment.status,
      }),
    );
    if (deployment.finished_at) {
      out.push(
        timelineEntry(deployment.status === 'rolled_back' ? 'rollback' : 'deployment', deployment.finished_at, {
          subject: deployment.id,
          label: `${deployment.service} ${deployment.version} ${deployment.status}`,
          service: deployment.service,
          environment: deployment.environment,
          status: deployment.status,
        }),
      );
    }
  }
  return out;
}

export function incidentEntries(incidents) {
  return (incidents ?? []).map((incident) =>
    timelineEntry('incident', incident.opened_at, {
      subject: incident.id,
      label: incident.title,
      service: incident.service,
      environment: incident.environment,
      status: incident.severity,
    }),
  );
}

export function buildTimeline({ releases, deployments, incidents, approvals }) {
  const entries = [
    ...releaseEntries(releases),
    ...deploymentEntries(deployments),
    ...incidentEntries(incidents),
    ...(approvals ?? []).map((approval) =>
      timelineEntry('approval', approval.decided_at ?? approval.requested_at, {
        subject: approval.id,
        label: `${approval.by} ${approval.state}`,
        status: approval.state,
      }),
    ),
  ];
  return sortBy(entries, (entry) => sortableInstant(entry.at), 'desc');
}

export function timelineFor(service, sources) {
  return buildTimeline(sources).filter((entry) => entry.service === undefined || entry.service === service);
}

export function groupByDay(entries) {
  const out = new Map();
  for (const entry of entries ?? []) {
    const day = String(entry.at ?? '').slice(0, 10);
    const bucket = out.get(day) ?? [];
    bucket.push(entry);
    out.set(day, bucket);
  }
  return [...out.entries()].map(([day, rows]) => ({ day, entries: rows })).sort((left, right) => (left.day < right.day ? 1 : -1));
}

export function timelineRows(entries) {
  return (entries ?? []).map((entry) => ({
    at: entry.at,
    kind: entry.kind,
    label: entry.label,
    service: entry.service ?? '',
    environment: entry.environment ?? '',
    status: entry.status ?? '',
  }));
}

export function timelineSpan(entries) {
  if ((entries ?? []).length === 0) return { from: undefined, to: undefined, minutes: 0 };
  const sorted = sortBy(entries, (entry) => sortableInstant(entry.at), 'asc');
  const from = sorted[0].at;
  const to = sorted[sorted.length - 1].at;
  return { from, to, minutes: diffMinutes(from, to) };
}

export function countKinds(entries) {
  const out = {};
  for (const entry of entries ?? []) out[entry.kind] = (out[entry.kind] ?? 0) + 1;
  return out;
}

export function timelineForRelease(release, sources) {
  return buildTimeline(sources).filter((entry) => entry.label?.includes(release.version) || entry.subject === release.id);
}

export function timelineWindow(entries, from, to) {
  return (entries ?? []).filter((entry) => new Date(entry.at).getTime() >= new Date(from).getTime() && new Date(entry.at).getTime() <= new Date(to).getTime());
}

export function latestEntry(entries) {
  return sortBy(entries ?? [], (entry) => sortableInstant(entry.at), 'desc')[0];
}

export function describeTimeline(entries) {
  const span = timelineSpan(entries);
  return { entries: (entries ?? []).length, kinds: countKinds(entries), minutes: span.minutes, from: span.from, to: span.to };
}

export function unmatchedKinds(entries, wanted) {
  const present = new Set((entries ?? []).map((entry) => entry.kind));
  return (wanted ?? TIMELINE_KINDS).filter((kind) => !present.has(kind));
}
