import { groupBy } from '../util/collections.js';
import { dayBucket } from '../util/time.js';

export function summaryFor(tenant, events, options = {}) {
  const groups = groupBy(events ?? [], (event) => dayBucket(event.at));
  const days = [...groups.entries()]
    .map(([day, rows]) => ({ day, events: rows.length }))
    .sort((left, right) => (left.day < right.day ? -1 : 1));
  const total = days.reduce((sum, day) => sum + day.events, 0);
  return {
    tenant,
    days,
    total,
    average: days.length === 0 ? 0 : Math.round((total / days.length) * 100) / 100,
    subject: options.subject ?? `Event summary for ${tenant}`,
  };
}

export function renderTextSummary(summary) {
  const lines = [summary.subject, ''];
  for (const day of summary.days) lines.push(`${day.day}: ${day.events} events`);
  lines.push('', `Total: ${summary.total} events over ${summary.days.length} day(s)`);
  return `${lines.join('\n')}\n`;
}

export function renderMarkdownSummary(summary) {
  const lines = [`# ${summary.subject}`, '', '| day | events |', '| --- | --- |'];
  for (const day of summary.days) lines.push(`| ${day.day} | ${day.events} |`);
  lines.push('', `Total: **${summary.total}** events.`);
  return `${lines.join('\n')}\n`;
}

export function recipientsOf(list) {
  return (list ?? [])
    .map((entry) => String(entry).trim())
    .filter((entry) => /^[^@\s]+@[^@\s]+\.[a-z]{2,}$/i.test(entry));
}

export function summarizeWeek(events, from, to) {
  const days = groupBy(events ?? [], (event) => dayBucket(event.at));
  const rows = [...days.entries()]
    .filter(([day]) => (from === undefined || day >= from) && (to === undefined || day <= to))
    .map(([day, bucket]) => ({ day, events: bucket.length }));
  return { from, to, days: rows.length, events: rows.reduce((total, row) => total + row.events, 0), rows };
}

export function weeklyDigest(events, options = {}) {
  const summary = summaryFor(options.tenant ?? 'all', events, options);
  const busiest = summary.days.reduce((best, day) => (best === undefined || day.events > best.events ? day : best), undefined);
  return {
    tenant: summary.tenant,
    days: summary.days.length,
    events: summary.totals?.events ?? 0,
    busiest: busiest ?? null,
    text: renderTextSummary(summary),
  };
}

export function digestRows(summary) {
  return (summary.days ?? []).map((day) => ({
    day: day.day,
    events: day.events,
    share: summary.totals?.events ? Math.round((day.events / summary.totals.events) * 10000) / 100 : 0,
  }));
}

export function summaryFooter(summary) {
  const rows = digestRows(summary);
  return {
    days: rows.length,
    events: rows.reduce((total, row) => total + row.events, 0),
    quiet_days: rows.filter((row) => row.events === 0).length,
  };
}

export function mergedSummaries(summaries) {
  const days = new Map();
  for (const summary of summaries ?? []) {
    for (const day of summary.days ?? []) {
      const current = days.get(day.day) ?? { day: day.day, events: 0 };
      current.events += day.events;
      days.set(day.day, current);
    }
  }
  const points = [...days.values()].sort((left, right) => (left.day < right.day ? -1 : 1));
  return { days: points, total: points.reduce((total, day) => total + day.events, 0) };
}
