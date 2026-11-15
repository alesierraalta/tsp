import { countsBy, groupBy, sortByKey } from '../store/query.js';
import { dayKey } from '../util/time.js';

export function tenantReport(tenants, users, options = {}) {
  const planCounts = countsBy(tenants, 'plan');
  const usersPerTenant = countsBy(users, 'tenant_id');
  const lines = [
    `# Tenant report ${options.day ?? dayKey(new Date())}`,
    '',
    `tenants: ${tenants.length}`,
    `users: ${users.length}`,
    '',
    '## plans',
  ];
  for (const [plan, count] of sortByKey(new Map(Object.entries(planCounts)))) {
    lines.push(`- ${plan}: ${count}`);
  }
  lines.push('', '## tenants by user count');
  const ranked = [...tenants]
    .map((tenant) => ({ slug: tenant.slug, plan: tenant.plan, users: usersPerTenant[tenant.id] ?? 0 }))
    .sort((left, right) => right.users - left.users || (left.slug < right.slug ? -1 : 1));
  for (const row of ranked) {
    lines.push(`- ${row.slug} (${row.plan}): ${row.users}`);
  }
  return `${lines.join('\n')}\n`;
}

export function usageReport(audits, options = {}) {
  const groups = groupBy(audits, 'action');
  const rows = [];
  for (const [action, entries] of groups.entries()) {
    rows.push({
      action,
      count: entries.length,
      last: entries.map((entry) => entry.at).sort().at(-1) ?? '',
    });
  }
  rows.sort((left, right) => right.count - left.count || (left.action < right.action ? -1 : 1));
  return rows.slice(0, options.limit ?? 25);
}

export function userReport(users) {
  return users
    .map((user) => ({
      email: user.email,
      tenant: user.tenant_id,
      roles: [...(user.roles ?? [])].sort().join('+'),
      active: user.active !== false,
    }))
    .sort((left, right) => (left.email < right.email ? -1 : 1));
}

export function summaryLine(report) {
  if (!Array.isArray(report)) return 'no rows';
  return `${report.length} row${report.length === 1 ? '' : 's'}`;
}

export function csvRows(report, fields) {
  const header = fields.join(',');
  const lines = report.map((row) => fields.map((field) => escapeCell(row[field])).join(','));
  return [header, ...lines].join('\n');
}

function escapeCell(value) {
  const text = value === undefined || value === null ? '' : String(value);
  if (/[",\n]/.test(text)) return `"${text.replace(/"/g, '""')}"`;
  return text;
}

export function totals(rows, field) {
  return rows.reduce((sum, row) => sum + Number(row[field] ?? 0), 0);
}

export function averages(rows, field) {
  if (rows.length === 0) return 0;
  return totals(rows, field) / rows.length;
}
