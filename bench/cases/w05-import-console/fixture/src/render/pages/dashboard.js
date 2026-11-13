import { renderStatGrid, renderCardGrid, renderAlertCard } from '../components/card.js';
import { renderBars, renderMoneyBars, renderSparkline } from '../components/chart.js';
import { renderTable } from '../components/table.js';
import { summaryLine } from '../../reports/summary.js';

export function renderDashboard(model) {
  const stats = [
    { label: 'Items', value: model.itemCount ?? 0, delta: model.itemDelta },
    { label: 'Open orders', value: model.openOrders ?? 0, delta: model.orderDelta },
    { label: 'Revenue', value: model.revenueCents ?? 0, money: true },
    { label: 'Refunds', value: model.refundCents ?? 0, money: true, trend: 'down' },
  ];
  const cards = [
    { title: 'Imports', body: summaryLine(model.imports ?? []), meta: 'last 7 days', href: '/imports' },
    { title: 'Items', body: summaryLine(model.items ?? []), href: '/items' },
    { title: 'Orders', body: summaryLine(model.orders ?? []), href: '/orders' },
  ];
  return [
    '<section class="dashboard">',
    renderStatGrid(stats, { currency: model.currency }),
    renderMoneyBars({
      data: model.revenueByMonth ?? [],
      title: 'Revenue by month',
    }),
    renderBars({
      data: model.ordersByStatus ?? [],
      title: 'Orders by status',
    }),
    renderSparkline((model.imports ?? []).map((entry) => entry.rows ?? 0)),
    renderCardGrid(cards),
    (model.alerts ?? []).map((alert) => renderAlertCard(alert)).join(''),
    renderTable({
      columns: [
        { key: 'when', label: 'When' },
        { key: 'what', label: 'Event' },
        { key: 'who', label: 'By' },
      ],
      rows: model.events ?? [],
      caption: 'Recent activity',
    }),
    '</section>',
  ].join('\n');
}

export function renderDashboardEmpty() {
  return [
    '<section class="dashboard empty">',
    '<h2>Nothing imported yet</h2>',
    '<p>Upload a CSV file to see the dashboard.</p>',
    '<a class="button" href="/imports">Import data</a>',
    '</section>',
  ].join('\n');
}

export function dashboardTitle(model) {
  const count = (model.imports ?? []).length;
  return count === 0 ? 'Dashboard' : `Dashboard (${count} imports)`;
}
