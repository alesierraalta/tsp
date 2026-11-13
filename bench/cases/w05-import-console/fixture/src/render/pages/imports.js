import { renderTable } from '../components/table.js';
import { renderUploadForm, renderFilterForm } from '../components/form.js';
import { renderNotice } from '../layout.js';

export function renderImportsPage(model) {
  const problems = model.problems ?? [];
  const problemTable =
    problems.length > 0
      ? renderTable({
          columns: [
            { key: 'row', label: 'Row', integer: true },
            { key: 'field', label: 'Column' },
            { key: 'message', label: 'Problem' },
          ],
          rows: problems,
          caption: 'Rejected rows',
        })
      : renderNotice('The last import had no problems.', { kind: 'ok' });
  return [
    '<section class="imports">',
    renderUploadForm({ datasets: model.datasets ?? [] }),
    problemTable,
    renderTable({
      columns: [
        { key: 'dataset', label: 'Dataset' },
        { key: 'rows', label: 'Rows', integer: true },
        { key: 'when', label: 'Imported' },
        { key: 'checksum', label: 'Checksum' },
      ],
      rows: model.history ?? [],
      caption: 'Import history',
    }),
    renderFilterForm({ action: '/imports', q: model.q, status: model.status, statuses: model.statuses }),
    '</section>',
  ].join('\n');
}

export function renderValidationPage(model) {
  const groups = model.groups ?? [];
  const details = groups.map(
    (group) => `<details${group.open ? ' open' : ''}><summary>${group.label}</summary><p>${group.message}</p></details>`,
  );
  return [
    '<section class="validation">',
    '<h2>Validation report</h2>',
    renderNotice(`${model.checked ?? 0} rows checked`, { kind: 'info' }),
    details.join(''),
    '</section>',
  ].join('\n');
}

export function renderImportResult(model) {
  const state = model.ok ? 'ok' : 'error';
  return [
    `<section class="import-result" data-state="${state}">`,
    `<h2>${model.title}</h2>`,
    `<p>${model.message}</p>`,
    model.rows === undefined ? '' : `<p class="rows">${model.rows} rows</p>`,
    '</section>',
  ].join('\n');
}

export function renderDatasetPicker(datasets, selected) {
  const options = datasets.map((entry) => {
    const value = typeof entry === 'string' ? entry : entry.value;
    const label = typeof entry === 'string' ? entry : entry.label;
    const current = String(value) === String(selected) ? ' selected' : '';
    return `<option value="${value}"${current}>${label}</option>`;
  });
  return `<select name="dataset">${options.join('')}</select>`;
}
