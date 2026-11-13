import { escapeText, attrPair } from './escaping.js';
import { section, empty } from './layout.js';
import { card } from './components.js';
import { table } from './tables.js';
import { renderStatusPill, renderKeyValue } from './partials.js';
import { formatNumber } from '../util/numbers.js';

export const READINESS_STATES = ['go', 'careful', 'hold'];

export function readinessOf(checks) {
  const list = checks ?? [];
  if (list.some((check) => check.state === 'hold')) return 'hold';
  if (list.some((check) => check.state === 'careful')) return 'careful';
  return 'go';
}

export function readinessReason(state) {
  const reasons = {
    go: 'Every gate passed; the release may be promoted.',
    careful: 'At least one gate wants a human to look before the promotion.',
    hold: 'At least one gate refuses the promotion.',
  };
  return reasons[state] ?? reasons.careful;
}

export function checkRows(view) {
  return [
    { gate: 'freeze', state: view.freeze?.active === true ? 'hold' : 'go', detail: view.freeze?.reason ?? 'no freeze' },
    { gate: 'approvals', state: (view.approvals ?? 0) >= (view.required ?? 0) ? 'go' : 'careful', detail: `${formatNumber(view.approvals ?? 0)} of ${formatNumber(view.required ?? 0)}` },
    { gate: 'incidents', state: (view.blocking ?? 0) > 0 ? 'hold' : 'go', detail: `${formatNumber(view.blocking ?? 0)} blocking` },
    { gate: 'policy', state: view.policy?.allowed === false ? 'hold' : 'go', detail: (view.policy?.reasons ?? [])[0] ?? 'allowed' },
    { gate: 'rollout', state: (view.steps ?? 0) > 0 ? 'go' : 'careful', detail: `${formatNumber(view.steps ?? 0)} steps` },
  ];
}

export function readinessTable(view) {
  return table(
    [
      { key: 'gate', label: 'Gate' },
      { key: 'state', label: 'State', render: (row) => renderStatusPill(row.state) },
      { key: 'detail', label: 'Detail' },
    ],
    checkRows(view),
  );
}

export function readinessPanel(view) {
  const state = readinessOf(checkRows(view));
  return section(
    'readiness',
    card(
      `Readiness: ${state}`,
      [readinessTable(view), `<p class="why">${escapeText(readinessReason(state))}</p>`].join(''),
    ),
  );
}

export function readinessBanner(state, text) {
  return `<div class="readiness ${escapeText(state)}" ${attrPair('data-readiness', state)}>${escapeText(text)}</div>`;
}

export function readinessSummary(view) {
  const rows = checkRows(view);
  return {
    state: readinessOf(rows),
    go: rows.filter((row) => row.state === 'go').length,
    careful: rows.filter((row) => row.state === 'careful').length,
    hold: rows.filter((row) => row.state === 'hold').length,
    gates: rows.length,
  };
}

export function blockedGates(view) {
  return checkRows(view).filter((row) => row.state === 'hold').map((row) => row.gate);
}

export function readinessDetails(view) {
  return section('readiness-details', renderKeyValue({ state: readinessOf(checkRows(view)), blocked: blockedGates(view).join(', ') || 'none' }));
}

export function canPromote(view) {
  return readinessOf(checkRows(view)) !== 'hold';
}

export function emptyReadiness() {
  return section('readiness', empty('No promotion is waiting.'));
}
