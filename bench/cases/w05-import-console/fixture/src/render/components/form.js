import { escapeAttr, escapeText } from '../escape.js';

export function renderField(field) {
  const name = escapeAttr(field.name);
  const id = escapeAttr(field.id ?? field.name);
  const value = escapeAttr(field.value ?? '');
  const required = field.required ? ' required' : '';
  const disabled = field.disabled ? ' disabled' : '';
  const describedBy = field.hint ? ` aria-describedby="${escapeAttr(`${field.name}-hint`)}"` : '';
  if (field.type === 'textarea') {
    return [
      `<label for="${id}">${escapeText(field.label ?? field.name)}</label>`,
      `<textarea id="${id}" name="${name}"${required}${disabled}${describedBy}>${escapeText(field.value ?? '')}</textarea>`,
      field.hint ? `<p id="${escapeAttr(`${field.name}-hint`)}" class="hint">${escapeText(field.hint)}</p>` : '',
      field.error ? `<p class="error">${escapeText(field.error)}</p>` : '',
    ].join('');
  }
  if (field.type === 'select') {
    const options = (field.options ?? [])
      .map((option) => {
        const selected = String(option.value) === String(field.value) ? ' selected' : '';
        return `<option value="${escapeAttr(option.value)}"${selected}>${escapeText(option.label)}</option>`;
      })
      .join('');
    return [
      `<label for="${id}">${escapeText(field.label ?? field.name)}</label>`,
      `<select id="${id}" name="${name}"${required}${disabled}${describedBy}>${options}</select>`,
      field.hint ? `<p id="${escapeAttr(`${field.name}-hint`)}" class="hint">${escapeText(field.hint)}</p>` : '',
      field.error ? `<p class="error">${escapeText(field.error)}</p>` : '',
    ].join('');
  }
  return [
    `<label for="${id}">${escapeText(field.label ?? field.name)}</label>`,
    `<input type="${escapeAttr(field.type ?? 'text')}" id="${id}" name="${name}" value="${value}"${required}${disabled}${describedBy}>`,
    field.hint ? `<p id="${escapeAttr(`${field.name}-hint`)}" class="hint">${escapeText(field.hint)}</p>` : '',
    field.error ? `<p class="error">${escapeText(field.error)}</p>` : '',
  ].join('');
}

export function renderForm(options) {
  const fields = (options.fields ?? []).map((field) => `<div class="field">${renderField(field)}</div>`);
  const errors = (options.errors ?? []).map(
    (entry) => `<li data-field="${escapeAttr(entry.field)}">${escapeText(entry.message)}</li>`,
  );
  const errorBlock = errors.length > 0 ? `<ul class="form-errors">${errors.join('')}</ul>` : '';
  return [
    `<form method="${escapeAttr(options.method ?? 'post')}" action="${escapeAttr(options.action ?? '')}"${options.enctype ? ` enctype="${escapeAttr(options.enctype)}"` : ''}>`,
    errorBlock,
    fields.join(''),
    `<button type="submit">${escapeText(options.submitLabel ?? 'Save')}</button>`,
    '</form>',
  ].join('');
}

export function renderUploadForm(options = {}) {
  return renderForm({
    action: options.action ?? '/imports',
    method: 'post',
    enctype: 'multipart/form-data',
    submitLabel: options.submitLabel ?? 'Import',
    fields: [
      { name: 'dataset', label: 'Dataset', type: 'select', options: options.datasets ?? [] },
      { name: 'file', label: 'CSV file', type: 'file', required: true },
      { name: 'note', label: 'Note', type: 'textarea', hint: 'Shown on the import history' },
    ],
  });
}

export function renderFilterForm(options = {}) {
  return renderForm({
    action: options.action ?? '',
    method: 'get',
    submitLabel: 'Filter',
    fields: [
      { name: 'q', label: 'Search', type: 'search', value: options.q ?? '' },
      { name: 'status', label: 'Status', type: 'select', value: options.status, options: options.statuses ?? [] },
    ],
  });
}

export function renderHidden(name, value) {
  return `<input type="hidden" name="${escapeAttr(name)}" value="${escapeAttr(value ?? '')}">`;
}

export function fieldFromColumn(column) {
  return {
    name: column.name,
    label: column.label ?? column.name,
    type: column.type === 'money' || column.type === 'integer' ? 'number' : 'text',
    required: Boolean(column.required),
    hint: column.hint,
  };
}
