function escapeMarkup(value) {
  return String(value).replaceAll("&", "&amp;").replaceAll("<", "&lt;").replaceAll(">", "&gt;").replaceAll('"', "&quot;");
}

export function renderText(value) {
  return escapeMarkup(value);
}

export function renderLink(label, href) {
  const safeLabel = escapeMarkup(label);
  const safeHref = escapeMarkup(href);
  return `<a href="${safeHref}">${safeLabel}</a>`;
}

export function renderColor(label, value) {
  const text = String(value);
  if (text.length === 0) {
    return `<span style="color:inherit">${escapeMarkup(label)}</span>`;
  }
  const safeValue = text.replaceAll(';', '\\3B ').replaceAll('"', '&quot;');
  return `<span style="color:${safeValue}">${escapeMarkup(label)}</span>`;
}
