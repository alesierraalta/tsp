export function asJSON(result) {
  return JSON.stringify({
    query: result.query.text,
    mode: result.query.mode,
    total: result.total,
    page: result.page,
    pages: result.pages,
    items: result.rows,
  }, null, 2);
}

export function asText(result) {
  const heading = result.query.empty ? 'All documents' : result.query.text;
  const lines = [heading, `${result.total} results`, ''];
  for (const row of result.rows) {
    lines.push(`${row.title} (${row.id})`);
    lines.push(row.excerpt);
    lines.push('');
  }
  lines.push(`Page ${result.page} of ${result.pages}`);
  return lines.join('\n');
}

export function pageLinks(result, base = '/search') {
  const links = {};
  const link = page => {
    const params = new URLSearchParams({ q: result.query.text, page: String(page) });
    return `${base}?${params}`;
  };
  if (result.page > 1) {
    links.previous = link(result.page - 1);
  }
  if (result.hasMore) {
    links.next = link(result.page + 1);
  }
  return links;
}

export function formatResult(result, format = 'json') {
  if (format === 'json') return asJSON(result);
  if (format === 'text') return asText(result);
  throw new Error('unsupported output format');
}
