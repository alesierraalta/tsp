export function entryView(entry, options = {}) {
  const view = {
    id: entry.id,
    type: entry.type,
    slug: entry.slug,
    locale: entry.locale,
    status: entry.status,
    version: entry.version,
    values: entry.values,
    created_at: entry.created_at,
    updated_at: entry.updated_at,
  };
  if (options.includePublished) view.published_at = entry.published_at;
  return view;
}

export function entryListView(entries, options = {}) {
  return entries.map((entry) => entryView(entry, options));
}

export function mediaView(described) {
  return {
    name: described.name,
    bytes: described.bytes,
    type: described.type,
    image: described.image,
  };
}

export function pluginView(manifest) {
  return {
    name: manifest.name,
    version: manifest.version,
    title: manifest.title,
    hooks: manifest.hooks,
    enabled: manifest.enabled,
  };
}

export function pageView(page) {
  return {
    items: page.items,
    total: page.total,
    limit: page.limit,
    offset: page.offset,
    has_more: page.has_more,
  };
}

export function statsView(stats) {
  return { ...stats, generated_at: new Date().toISOString() };
}

export function errorView(error) {
  return typeof error?.toJSON === 'function' ? error.toJSON() : { error: 'internal_error', message: 'internal error' };
}
