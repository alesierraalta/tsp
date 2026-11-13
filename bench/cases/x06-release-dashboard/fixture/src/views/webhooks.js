import { page, section, twoColumn, empty } from './layout.js';
import { card, statRow, link } from './components.js';
import { deliveriesTable, emptyTable, incidentColumns, table } from './tables.js';
import { renderHostList, renderKeyValue, renderStatusPill, renderBreadcrumbs, renderTicketLink } from './partials.js';
import { escapeText } from './escaping.js';
import { formatNumber, percentOf } from '../util/numbers.js';
import { formatDuration } from '../util/time.js';

export function deliveryDetail(delivery, options = {}) {
  return [
    section(
      'delivery',
      renderKeyValue({
        event: delivery.event,
        host: delivery.host,
        state: delivery.state,
        attempts: delivery.attempts,
        queued_at: delivery.queued_at,
        posted_at: delivery.posted_at ?? 'never',
      }),
    ),
    section('payload', `<pre class="payload">${escapeText(JSON.stringify(delivery.payload ?? {}, null, 2))}</pre>`),
    options.retries ? section('retries', `<ul class="retries">${options.retries.map((row) => `<li>attempt ${escapeText(row.attempt)} — ${escapeText(row.error ?? row.at)}</li>`).join('')}</ul>`) : '',
  ]
    .filter(Boolean)
    .join('');
}

export function subscriptionsPanel(subscriptions) {
  if ((subscriptions ?? []).length === 0) return empty('No subscriptions yet.');
  const rows = subscriptions.map((subscription) => ({
    id: subscription.id,
    host: subscription.host,
    events: subscription.events.join(', '),
    enabled: subscription.enabled,
  }));
  return section(
    'subscriptions',
    table(
      [
        { key: 'host', label: 'Host', render: (row) => renderTicketLink(row.host, `/webhooks?host=${encodeURIComponent(row.host)}`) },
        { key: 'events', label: 'Events' },
        { key: 'enabled', label: 'Enabled', render: (row) => renderStatusPill(row.enabled ? 'enabled' : 'disabled') },
      ],
      rows,
    ),
  );
}

export function deliveryStatsPanel(stats, deliveries) {
  const summary = deliveries ?? [];
  return section(
    'delivery-stats',
    statRow([
      { label: 'deliveries', value: stats.deliveries ?? 0, format: formatNumber },
      { label: 'posted', value: stats.byState?.posted ?? 0, format: formatNumber },
      { label: 'failed', value: stats.byState?.failed ?? 0, format: formatNumber },
      { label: 'success', value: percentOf(stats.byState?.posted ?? 0, summary.length || 1), format: (value) => `${value}%` },
    ]),
  );
}

export function webhooksBody(view) {
  const left = [subscriptionsPanel(view.subscriptions ?? []), deliveryStatsPanel(view.stats ?? {}, view.deliveries ?? [])].join('');
  const right = [
    section('deliveries', (view.deliveries ?? []).length === 0 ? emptyTable('No deliveries yet.') : deliveriesTable(view.deliveries)),
    section('hosts', renderHostList(view.hosts ?? {})),
  ].join('');
  return twoColumn(left, right);
}

export function webhooksPage(view) {
  return page({
    title: view.title ?? 'Webhooks',
    section: 'webhooks',
    tenant: view.tenant,
    body: [renderBreadcrumbs([{ label: 'Webhooks' }]), webhooksBody(view)].join(''),
  });
}

export function deliveryPage(delivery, options = {}) {
  return page({
    title: `Delivery ${delivery.id}`,
    section: 'webhooks',
    tenant: options.tenant,
    body: [renderBreadcrumbs([{ label: 'Webhooks', href: '/webhooks' }, { label: delivery.id }]), deliveryDetail(delivery, options)].join(''),
  });
}

export function retryNotice(policy) {
  return card(
    'Retry policy',
    `<p>${formatNumber(policy.attempts)} attempts, ${formatDuration(policy.baseSeconds * 1000)} base delay, at most ${formatDuration(policy.maxSeconds * 1000)}.</p>`,
    { footer: link('policy', '/webhooks/policy') },
  );
}

export function incidentSummaryPanel(incidents) {
  const open = (incidents ?? []).filter((incident) => incident.state === 'open');
  if (open.length === 0) return empty('No open incidents.');
  return table(incidentColumns(), open);
}
