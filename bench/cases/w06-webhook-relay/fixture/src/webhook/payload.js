import { fingerprint } from '../util/ids.js';
import { canonicalJson, parseJson, redactJson } from '../util/json.js';

export const MAX_PAYLOAD_BYTES = 64 * 1024;

export function payloadBytes(payload) {
  return Buffer.byteLength(JSON.stringify(payload ?? null));
}

export function assertPayloadSize(payload, limit = MAX_PAYLOAD_BYTES) {
  const size = payloadBytes(payload);
  if (size > limit) throw new TypeError(`a payload holds at most ${limit} bytes, got ${size}`);
  return size;
}

export function envelope(event, payload, meta = {}) {
  assertPayloadSize(payload);
  return {
    event: String(event ?? 'generic'),
    id: meta.id ?? fingerprint([event, canonicalJson(payload)]),
    sent_at: new Date(meta.now ?? Date.now()).toISOString(),
    topic: meta.topic,
    data: payload,
  };
}

export function digestOf(payload) {
  return fingerprint([canonicalJson(payload ?? null)]);
}

export function parseEnvelope(text) {
  const parsed = parseJson(text, undefined);
  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
    throw new TypeError('an envelope must be a JSON object');
  }
  if (typeof parsed.event !== 'string' || parsed.data === undefined) {
    throw new TypeError('an envelope needs an event and data');
  }
  return parsed;
}

export function redactEnvelope(value) {
  return redactJson(value, ['secret', 'token', 'password', 'authorization', 'card']);
}

export function sameEnvelope(left, right) {
  return digestOf(left) === digestOf(right);
}

export function envelopeSummary(value) {
  const event = value?.event ?? 'unknown';
  const id = value?.id ?? 'none';
  return `${event} ${id} ${payloadBytes(value?.data ?? {})} bytes`;
}
