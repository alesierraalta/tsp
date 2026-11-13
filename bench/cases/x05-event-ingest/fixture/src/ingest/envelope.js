import { eventId } from '../util/ids.js';
import { nowIso } from '../util/time.js';

export const SOURCES = ['web', 'ios', 'android', 'server'];

export function normalizeEnvelope(input = {}) {
  const source = SOURCES.includes(input.source) ? input.source : 'web';
  const events = Array.isArray(input.events) ? input.events : [];
  return {
    batch: String(input.batch ?? eventId('batch')),
    source,
    sent_at: input.sent_at ?? nowIso(),
    events: events.map((event) => ({
      ...event,
      context: { ...(event.context ?? {}), source },
    })),
  };
}

export function envelopeSize(envelope) {
  return Buffer.byteLength(JSON.stringify(envelope), 'utf8');
}

export function envelopeSummary(envelope) {
  const names = {};
  for (const event of envelope.events) names[event.name] = (names[event.name] ?? 0) + 1;
  return {
    batch: envelope.batch,
    source: envelope.source,
    events: envelope.events.length,
    names,
    bytes: envelopeSize(envelope),
  };
}

export function splitEnvelope(envelope, size = 500) {
  const out = [];
  for (let index = 0; index < envelope.events.length; index += size) {
    out.push({ ...envelope, batch: `${envelope.batch}-${out.length + 1}`, events: envelope.events.slice(index, index + size) });
  }
  return out;
}

export function mergeEnvelopes(envelopes) {
  const events = (envelopes ?? []).flatMap((envelope) => envelope.events);
  return {
    batch: envelopes?.[0]?.batch ?? eventId('batch'),
    source: envelopes?.[0]?.source ?? 'web',
    sent_at: envelopes?.[0]?.sent_at ?? nowIso(),
    events,
  };
}

export function validateEnvelope(envelope) {
  const problems = [];
  if (!Array.isArray(envelope?.events)) problems.push('an envelope needs events');
  for (const [index, event] of (envelope?.events ?? []).entries()) {
    if (typeof event?.name !== 'string' || event.name === '') problems.push(`event ${index + 1} has no name`);
  }
  return { ok: problems.length === 0, problems };
}

export const ENVELOPE_VERSION = 2;

// A sender that has been upgraded stamps the envelope version it speaks. Version 1 envelopes carry the
// source on the batch and version 2 carries it on every event, so reading a batch has to accept both and
// report which one it read instead of assuming the newer shape.
export function versionOf(envelope) {
  return Number(envelope?.version ?? 1) === ENVELOPE_VERSION ? ENVELOPE_VERSION : 1;
}

export function upgradeEnvelope(envelope) {
  const version = versionOf(envelope);
  const events = (envelope?.events ?? []).map((event) =>
    version === ENVELOPE_VERSION ? event : { ...event, context: { ...(event.context ?? {}), source: envelope.source ?? 'web' } },
  );
  return { ...envelope, version: ENVELOPE_VERSION, events };
}

export function downgradeEnvelope(envelope) {
  return { ...envelope, version: 1, events: (envelope?.events ?? []).map((event) => ({ ...event, context: { ...(event.context ?? {}) } })) };
}

export function envelopeBytesByEvent(envelope) {
  return (envelope?.events ?? []).map((event) => ({ name: event.name, bytes: Buffer.byteLength(JSON.stringify(event), 'utf8') }));
}

export function acceptedEnvelope(envelope, limit) {
  const cap = Math.max(0, Math.trunc(Number(limit) || 0));
  const events = (envelope?.events ?? []).slice(0, cap);
  return {
    ...envelope,
    events,
    dropped: Math.max(0, (envelope?.events ?? []).length - events.length),
    version: versionOf(envelope),
  };
}
