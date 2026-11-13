import { WEBHOOK_EVENTS } from './subscriptions.js';

export function buildPayload(event, data = {}, options = {}) {
  if (!WEBHOOK_EVENTS.includes(event)) throw new TypeError(`unknown webhook event ${event}`);
  return {
    event,
    at: options.at ?? new Date().toISOString(),
    tenant: options.tenant ?? 'public',
    data,
  };
}

export function releasePayload(release, options = {}) {
  return buildPayload('release.created', {
    release: release.id,
    version: release.version,
    service: release.service,
    environment: release.environment,
    status: release.status,
    channel: release.channel,
  }, options);
}

export function deploymentPayload(deployment, options = {}) {
  const event = deployment.status === 'rolled_back' ? 'deployment.rolled_back' : options.event ?? 'deployment.started';
  return buildPayload(event, {
    deployment: deployment.id,
    release: deployment.release,
    service: deployment.service,
    environment: deployment.environment,
    stage: deployment.stage,
    status: deployment.status,
  }, options);
}

export function incidentPayload(incident, options = {}) {
  return buildPayload('incident.opened', {
    incident: incident.id,
    title: incident.title,
    severity: incident.severity,
    service: incident.service,
    environment: incident.environment,
  }, options);
}

export function describePayload(payload) {
  return {
    event: payload.event,
    tenant: payload.tenant,
    rows: Object.keys(payload.data ?? {}).length,
    bytes: Buffer.byteLength(JSON.stringify(payload), 'utf8'),
  };
}

export function payloadEvents() {
  return [...WEBHOOK_EVENTS];
}

export function matchesEvent(payload, wanted) {
  return wanted === undefined ? true : payload.event === wanted;
}
