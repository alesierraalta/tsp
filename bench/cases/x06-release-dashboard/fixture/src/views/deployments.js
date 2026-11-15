import { page, section, twoColumn, empty, notice } from './layout.js';
import { card, progress, statRow, summaryLine, avatarStack } from './components.js';
import { deploymentColumns, table, emptyTable } from './tables.js';
import { renderEnvironmentTag, renderKeyValue, renderReleaseTag, renderServiceTag, renderStatusPill, renderDuration, renderBreadcrumbs, renderProgressBar } from './partials.js';
import { escapeText } from './escaping.js';
import { formatNumber, percentOf } from '../util/numbers.js';
import { diffMinutes, formatDuration } from '../util/time.js';

export function stageList(deployment) {
  const stages = (deployment.plan ?? []).map((stage) => {
    const state = stage.index < deployment.stage_index ? 'done' : stage.index === deployment.stage_index ? 'current' : 'pending';
    const approvals = stage.approvals > 0 ? `<span class="approvals">${formatNumber(stage.approvals)} approval(s)</span>` : '';
    return `<li class="stage ${state}" ${`data-stage="${escapeText(stage.name)}"`}>${escapeText(stage.label)}${approvals}</li>`;
  });
  return `<ol class="stages">${stages.join('')}</ol>`;
}

export function approvalTrail(approvals) {
  if ((approvals ?? []).length === 0) return empty('No approvals recorded yet.');
  return `<ul class="approvals">${approvals
    .map((approval) => `<li>${escapeText(approval.by)} — ${renderStatusPill(approval.state)}${approval.note ? ` <span class="note">${escapeText(approval.note)}</span>` : ''}</li>`)
    .join('')}</ul>`;
}

export function deploymentHeader(deployment, options = {}) {
  return [
    '<header class="deployment">',
    `<h2>${renderServiceTag({ name: deployment.service })} ${renderReleaseTag({ version: deployment.version })}</h2>`,
    `<p>${renderEnvironmentTag({ name: deployment.environment })} ${renderStatusPill(deployment.status)} ${renderDuration(diffMinutes(deployment.started_at, deployment.finished_at ?? new Date()))}</p>`,
    summaryLine(options.summary ?? `${deployment.service} is at stage ${deployment.stage}`),
    '</header>',
  ].join('');
}

export function deploymentBody(deployment, options = {}) {
  const left = [
    section('stages', stageList(deployment)),
    section('approvals', approvalTrail(options.approvals ?? deployment.approvals ?? [])),
  ].join('');
  const right = [
    section(
      'details',
      renderKeyValue({
        release: deployment.release,
        environment: deployment.environment,
        stage: deployment.stage,
        status: deployment.status,
        started_by: deployment.started_by,
        started_at: deployment.started_at,
      }),
    ),
    section('rollout', renderProgressBar(percentOf(deployment.stage_index + 1, (deployment.plan ?? []).length || 1))),
  ].join('');
  return twoColumn(left, right);
}

export function deploymentPage(deployment, options = {}) {
  return page({
    title: `${deployment.service} ${deployment.version} → ${deployment.environment}`,
    subtitle: options.subtitle,
    section: 'deployments',
    tenant: options.tenant ?? deployment.tenant,
    body: [
      renderBreadcrumbs([{ label: 'Deployments', href: '/deployments' }, { label: deployment.id }]),
      options.notice ? notice(options.notice, options.noticeKind) : '',
      deploymentHeader(deployment, options),
      deploymentBody(deployment, options),
    ]
      .filter(Boolean)
      .join(''),
  });
}

export function deploymentsPage(deployments, options = {}) {
  const finished = (deployments ?? []).filter((deployment) => deployment.status === 'succeeded').length;
  return page({
    title: options.title ?? 'Deployments',
    section: 'deployments',
    tenant: options.tenant,
    body: [
      statRow([
        { label: 'deployments', value: (deployments ?? []).length, format: formatNumber },
        { label: 'succeeded', value: finished, format: formatNumber },
        { label: 'open', value: (deployments ?? []).length - finished, format: formatNumber },
      ]),
      section('list', (deployments ?? []).length === 0 ? emptyTable('No deployments recorded.') : table(deploymentColumns(), deployments)),
    ].join(''),
  });
}

export function approvalQueue(deployments, approvals) {
  const waiting = (deployments ?? []).filter((deployment) => deployment.status === 'awaiting_approval');
  if (waiting.length === 0) return card('Approvals', empty('Nothing waits for an approval.'));
  const body = waiting
    .map((deployment) => {
      const pending = (approvals ?? []).filter((approval) => approval.deployment === deployment.id && approval.state === 'pending');
      return `<li>${escapeText(deployment.service)} ${escapeText(deployment.version)} — ${renderEnvironmentTag({ name: deployment.environment })} (${formatNumber(pending.length)} pending)</li>`;
    })
    .join('');
  return card('Approvals', `<ul class="queue">${body}</ul>`);
}

export function ownersStrip(owners) {
  return avatarStack(owners ?? []);
}

export function stageDurationTable(deployment) {
  const rows = (deployment.plan ?? []).map((stage) => ({
    stage: stage.name,
    timeout: formatDuration(stage.timeout_minutes * 60000),
    manual: stage.manual ? 'yes' : 'no',
  }));
  return table(
    [
      { key: 'stage', label: 'Stage' },
      { key: 'timeout', label: 'Timeout' },
      { key: 'manual', label: 'Manual' },
    ],
    rows,
  );
}

export function deploymentProgress(deployment) {
  return progress(deployment.stage_index ?? 0, (deployment.plan ?? []).length || 1);
}

export function stageCount(deployment) {
  return (deployment.plan ?? []).length;
}
