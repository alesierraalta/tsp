import { escapeText, attrPair, element, classNames } from './escaping.js';
import { titleCase, truncate, initials } from '../util/text.js';
import { formatNumber, percentOf } from '../util/numbers.js';

export function badge(text, kind = 'neutral') {
  return `<span class="badge ${classNames(kind)}">${escapeText(text)}</span>`;
}

export function button(label, options = {}) {
  const attributes = {
    class: classNames('button', options.variant ?? 'default'),
    type: options.type ?? 'button',
    ...(options.name ? { name: options.name } : {}),
    ...(options.value === undefined ? {} : { value: options.value }),
    ...(options.disabled ? { disabled: true } : {}),
  };
  return element('button', attributes, escapeText(label));
}

export function link(text, href, options = {}) {
  return element('a', { href, class: options.class ?? 'link' }, escapeText(text));
}

export function card(titleText, body, options = {}) {
  return [
    '<article class="card">',
    `<header><h2>${escapeText(titleText)}</h2>${options.action ?? ''}</header>`,
    `<div class="body">${body}</div>`,
    options.footer ? `<footer>${options.footer}</footer>` : '',
    '</article>',
  ]
    .filter(Boolean)
    .join('');
}

export function stat(label, value, options = {}) {
  const formatter = options.format ?? ((input) => String(input));
  const delta = options.delta === undefined ? '' : `<span class="delta ${options.delta >= 0 ? 'up' : 'down'}">${escapeText(String(options.delta))}</span>`;
  return `<div class="stat" ${attrPair('data-stat', label)}><span class="label">${escapeText(label)}</span><strong>${escapeText(formatter(value))}</strong>${delta}</div>`;
}

export function statRow(stats) {
  return `<div class="stats">${(stats ?? [])
    .map((entry) => stat(entry.label, entry.value, { format: entry.format, delta: entry.delta }))
    .join('')}</div>`;
}

export function avatar(name) {
  return `<span class="avatar" ${attrPair('data-owner', name)}>${escapeText(initials(name) || '?')}</span>`;
}

export function avatarStack(names) {
  return `<span class="avatars">${(names ?? []).map((name) => avatar(name)).join('')}</span>`;
}

export function progress(step, total) {
  const percent = percentOf(step, total);
  return `<div class="steps">${Array.from({ length: total }, (_, index) => {
    const state = index < step ? 'done' : index === step ? 'current' : 'pending';
    return `<span class="step ${state}" ${attrPair('data-step', index + 1)}></span>`;
  }).join('')}<span class="steps-label">${escapeText(`${percent}%`)}</span></div>`;
}

export function sparkline(values) {
  const numbers = (values ?? []).map((value) => Number(value) || 0);
  const peak = Math.max(1, ...numbers);
  const bars = numbers.map((value) => `<span style="height:${Math.round((value / peak) * 100)}%"></span>`).join('');
  return `<div class="sparkline" title="${escapeText(String(numbers.length))} points">${bars}</div>`;
}

export function metricRow(name, value, options = {}) {
  return `<tr ${attrPair('data-metric', name)}><th scope="row">${escapeText(titleCase(name.replace(/_/g, ' ')))}</th><td>${escapeText(options.format ? options.format(value) : formatNumber(value))}</td><td>${escapeText(options.note ?? '')}</td></tr>`;
}

export function summaryLine(text) {
  return `<p class="summary">${escapeText(truncate(text, 200))}</p>`;
}

export function legend(entries) {
  return `<ul class="legend">${(entries ?? [])
    .map((entry) => `<li><span class="swatch ${classNames(entry.kind)}"></span>${escapeText(entry.label)}</li>`)
    .join('')}</ul>`;
}
