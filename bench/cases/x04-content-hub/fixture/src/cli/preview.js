export const PREVIEWS = [
  { id: 'article-card', template: 'article-card.html', width: 480, height: 320 },
  { id: 'entry-row', template: 'entry-row.html', width: 960, height: 120 },
  { id: 'media-grid', template: 'media-grid.html', width: 720, height: 360 },
  { id: 'search-result', template: 'search-result.html', width: 960, height: 96 },
];

export const THEMES = {
  console: { label: 'Console', accent: '#2a6df4', density: 'compact' },
  print: { label: 'Print', accent: '#111111', density: 'comfortable' },
  minimal: { label: 'Minimal', accent: '#6b7280', density: 'comfortable' },
};

export function cliTheme(name = 'console') {
  const theme = THEMES[name];
  if (!theme) throw new TypeError(`unknown theme ${name}`);
  return theme;
}

export function listPreviews() {
  return PREVIEWS.map((preview) => ({ id: preview.id, width: preview.width, height: preview.height }));
}

export function previewById(id) {
  return PREVIEWS.find((preview) => preview.id === id);
}

export function previewForType(apiId) {
  return apiId === 'article' ? previewById('article-card') : previewById('entry-row');
}

export function themeClasses(theme, density) {
  return ['theme', `theme-${theme}`, `density-${density ?? cliTheme(theme).density}`].join(' ');
}

export function previewSummary() {
  return {
    count: PREVIEWS.length,
    largest: PREVIEWS.reduce((best, preview) => (preview.width > best.width ? preview : best), PREVIEWS[0]),
  };
}
