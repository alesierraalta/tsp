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
