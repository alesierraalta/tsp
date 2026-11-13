import { escapeText, escapeAttr, attrPair, element, classNames } from './escaping.js';
import { titleCase, truncate, pluralize } from '../util/text.js';
import { formatDuration } from '../util/time.js';

// The partials write the dashboard's stored names into markup. A name that arrives from a service record, a
// release or an owner is written into an attribute through attrPair, and the same value is written into the
// element text through escapeText.

export function renderServiceTag(service = {}) {
  const name = String(service.name ?? 'service');
  return `<span class="tag service" ${attrPair('data-service', name)}>${escapeText(name)}</span>`;
}

export function renderOwnerTag(owner) {
  return `<span class="tag owner" ${attrPair('data-owner', owner)}>${escapeText(owner)}</span>`;
}

export function renderReleaseTag(release = {}) {
  const version = String(release.version ?? '0.0.0');
  const channel = String(release.channel ?? 'stable');
  return `<span class=${attrPair('data-version', version)} data-channel="${escapeAttr(channel)}">${escapeText(version)}</span>`;
}

export function renderEnvironmentTag(environment = {}) {
  const label = String(environment.label ?? environment.name ?? 'unknown');
  return `<span class="tag environment" ${attrPair('data-environment', label)}>${escapeText(titleCase(label))}</span>`;
}

export function renderStatusPill(status) {
  const value = String(status ?? 'unknown');
  return `<span class="pill ${classNames('status', value)}" ${attrPair('data-status', value)}>${escapeText(titleCase(value.replace(/_/g, ' ')))}</span>`;
}

export function renderSeverityBadge(severity) {
  const value = String(severity ?? 'sev4');
  return `<span class="badge severity" ${attrPair('data-severity', value)}>${escapeText(value.toUpperCase())}</span>`;
}

export function renderDuration(minutes) {
  const value = Number(minutes) || 0;
  return `<span class="duration" ${attrPair('data-minutes', value)}>${escapeText(formatDuration(value * 60000))}</span>`;
}

export function renderProgressBar(percent) {
  const value = Math.max(0, Math.min(100, Math.round(Number(percent) || 0)));
  return `<div class="progress" role="progressbar" aria-valuenow="${value}" aria-valuemin="0" aria-valuemax="100"><span style="width:${value}%"></span></div>`;
}

export function renderChangelog(entries) {
  const rows = (entries ?? []).map((entry) => {
    const text = typeof entry === 'string' ? entry : entry.summary;
    const kind = typeof entry === 'string' ? 'chore' : entry.kind;
    return `<li ${attrPair('data-kind', kind)}>${escapeText(truncate(text, 160))}</li>`;
  });
  return rows.length === 0 ? '<p class="empty">No changes recorded.</p>' : `<ul class="changelog">${rows.join('')}</ul>`;
}

export function renderHostList(hosts) {
  const entries = Object.entries(hosts ?? {});
  if (entries.length === 0) return '<p class="empty">No deliveries yet.</p>';
  const rows = entries.map(([host, count]) => `<li ${attrPair('data-host', host)}>${escapeText(host)} — ${pluralize(count, 'delivery', 'deliveries')}</li>`);
  return `<ul class="hosts">${rows.join('')}</ul>`;
}

export function renderEmptyState(text) {
  return element('p', { class: 'empty' }, escapeText(text));
}

export function renderKeyValue(pairs) {
  const rows = Object.entries(pairs ?? {}).map(
    ([name, value]) => `<dt>${escapeText(name)}</dt><dd ${attrPair('data-field', name)}>${escapeText(value)}</dd>`,
  );
  return `<dl class="kv">${rows.join('')}</dl>`;
}

export function renderBreadcrumbs(trail) {
  return `<ol class="breadcrumbs">${(trail ?? [])
    .map((entry) => (entry.href ? `<li><a href="${escapeAttr(entry.href)}">${escapeText(entry.label)}</a></li>` : `<li>${escapeText(entry.label)}</li>`))
    .join('')}</ol>`;
}

export function renderTicketLink(reference, href) {
  if (!reference) return '';
  return `<a class="ticket" href="${escapeAttr(href)}">${escapeText(reference)}</a>`;
}

export function renderFreezeBanner(freeze) {
  if (!freeze) return '';
  return `<aside class="banner freeze" ${attrPair('data-freeze', freeze.id)}><strong>Change freeze</strong> ${escapeText(freeze.reason)} until ${escapeText(freeze.to)}</aside>`;
}

export function renderApprovalState(state) {
  return `<span class="approval ${classNames(state)}" ${attrPair('data-state', state)}>${escapeText(titleCase(state))}</span>`;
}

export function renderPolicyList(rows) {
  if ((rows ?? []).length === 0) return '<p class="empty">No policies configured.</p>';
  const items = rows
    .map((policy) => `<li ${attrPair('data-policy', policy.id)}>${escapeText(policy.kind)} in ${escapeText(policy.environment)} needs ${escapeText(policy.minimum_role)} (${pluralize(policy.approvals, 'approval')})</li>`)
    .join('');
  return `<ul class="policies">${items}</ul>`;
}

export function renderChannelTag(channel) {
  return `<span class="tag channel" ${attrPair('data-channel', channel)}>${escapeText(channel)}</span>`;
}

export function renderDeliveryState(state) {
  return `<span class="state ${classNames(state)}">${escapeText(state)}</span>`;
}

export function renderAttempts(attempts) {
  return `<span class="attempts" ${attrPair('data-attempts', attempts)}>${pluralize(Number(attempts) || 0, 'attempt')}</span>`;
}

export function renderLinkList(entries) {
  if ((entries ?? []).length === 0) return '<p class="empty">Nothing linked.</p>';
  return `<ul class="links">${entries
    .map((entry) => `<li><a href="${escapeAttr(entry.href)}">${escapeText(entry.label)}</a></li>`)
    .join('')}</ul>`;
}
