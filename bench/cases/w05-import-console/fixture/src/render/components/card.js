import { formatCents } from '../../money/decimal.js';
import { escapeAttr, escapeText, escapeUrl } from '../escape.js';

export function renderCard(card) {
  const title = escapeText(card.title ?? '');
  const body = escapeText(card.body ?? '');
  const tag = card.href ? 'a' : 'div';
  const hrefAttr = card.href ? ` href="${escapeAttr(escapeUrl(card.href))}"` : '';
  const meta = card.meta ? `<p class="meta">${escapeText(card.meta)}</p>` : '';
  return `<${tag} class="card" data-kind="${escapeAttr(card.kind ?? 'plain')}"${hrefAttr}><h2>${title}</h2><p>${body}</p>${meta}</${tag}>`;
}

export function renderStatCard(stat, options = {}) {
  const currency = options.currency ?? 'USD';
  const value = stat.money ? formatCents(stat.value ?? 0, currency) : String(stat.value ?? 0);
  const delta = stat.delta === undefined ? '' : `<span class="delta">${escapeText(formatDelta(stat.delta))}</span>`;
  return [
    `<div class="stat" data-trend="${escapeAttr(stat.trend ?? 'flat')}">`,
    `<span class="label">${escapeText(stat.label)}</span>`,
    `<strong class="value">${escapeText(value)}</strong>`,
    delta,
    '</div>',
  ].join('');
}

export function formatDelta(value) {
  const number = Number(value);
  if (!Number.isFinite(number)) return '';
  const sign = number > 0 ? '+' : '';
  return `${sign}${number.toFixed(1)}%`;
}

export function renderCardGrid(cards, options = {}) {
  const rendered = cards.map((card) => renderCard(card, options));
  return `<section class="cards">${rendered.join('')}</section>`;
}

export function renderStatGrid(stats, options = {}) {
  const rendered = stats.map((stat) => renderStatCard(stat, options));
  return `<section class="stats">${rendered.join('')}</section>`;
}

export function renderProfileCard(profile) {
  const roles = (profile.roles ?? []).map((role) => `<li>${escapeText(role)}</li>`).join('');
  return [
    '<article class="profile">',
    `<h2>${escapeText(profile.name)}</h2>`,
    `<p><a href="${escapeAttr(escapeUrl(`mailto:${profile.email ?? ''}`))}">${escapeText(profile.email ?? '')}</a></p>`,
    `<ul class="roles">${roles}</ul>`,
    '</article>',
  ].join('');
}

export function renderAlertCard(alert) {
  return [
    `<article class="alert" data-severity="${escapeAttr(alert.severity ?? 'info')}">`,
    `<h3>${escapeText(alert.title ?? '')}</h3>`,
    `<p>${escapeText(alert.message ?? '')}</p>`,
    alert.href ? `<a href="${escapeAttr(escapeUrl(alert.href))}">${escapeText(alert.label ?? 'Open')}</a>` : '',
    '</article>',
  ].join('');
}
