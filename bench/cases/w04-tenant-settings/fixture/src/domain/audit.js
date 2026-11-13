import { newId } from '../util/ids.js';

export function recordAudit(store, entry) {
  if (!store) throw new TypeError('a store is required');
  const row = {
    id: newId('aud'),
    tenant_id: String(entry.tenantId ?? ''),
    actor: String(entry.actor ?? 'system'),
    action: String(entry.action ?? 'unknown'),
    target: String(entry.target ?? ''),
    at: new Date(entry.at ?? Date.now()).toISOString(),
  };
  return store.insert('audits', row);
}

export function listAudits(store, tenantId, options = {}) {
  const rows = store.find('audits', (row) => row.tenant_id === tenantId);
  const sorted = rows.sort((left, right) => (left.at < right.at ? 1 : -1));
  return sorted.slice(0, options.limit ?? 50);
}

export function auditActions(store) {
  const actions = new Set(store.all('audits').map((row) => row.action));
  return [...actions].sort();
}

export function auditsFor(store, target) {
  return store.find('audits', (row) => row.target === target);
}
