import { escapeText } from '../templates/escaping.js';

export function sitemap(entries, options = {}) {
  const base = String(options.base ?? 'https://example.invalid').replace(/\/$/, '');
  const lines = ['<?xml version="1.0" encoding="UTF-8"?>', '<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">'];
  for (const entry of entries) {
    if (entry.status !== 'published') continue;
    lines.push('  <url>');
    lines.push(`    <loc>${escapeText(`${base}/${entry.slug}`)}</loc>`);
    if (entry.updated_at) lines.push(`    <lastmod>${escapeText(entry.updated_at.slice(0, 10))}</lastmod>`);
    lines.push(`    <changefreq>${options.changefreq ?? 'weekly'}</changefreq>`);
    lines.push(`    <priority>${options.priority ?? '0.6'}</priority>`);
    lines.push('  </url>');
  }
  lines.push('</urlset>');
  return `${lines.join('\n')}\n`;
}

export function rssFeed(entries, options = {}) {
  const base = String(options.base ?? 'https://example.invalid').replace(/\/$/, '');
  const items = entries
    .filter((entry) => entry.status === 'published')
    .slice(0, options.limit ?? 20)
    .map((entry) => {
      const title = escapeText(entry.values?.title ?? entry.slug);
      const link = escapeText(`${base}/${entry.slug}`);
      const body = escapeText(String(entry.values?.body ?? ''));
      return `    <item>\n      <title>${title}</title>\n      <link>${link}</link>\n      <description>${body.slice(0, 400)}</description>\n    </item>`;
    });
  return [
    '<?xml version="1.0" encoding="UTF-8"?>',
    '<rss version="2.0">',
    '  <channel>',
    `    <title>${escapeText(options.title ?? 'Content hub')}</title>`,
    `    <link>${escapeText(base)}</link>`,
    ...items,
    '  </channel>',
    '</rss>',
    '',
  ].join('\n');
}

export function sitemapIndex(files, options = {}) {
  const base = String(options.base ?? 'https://example.invalid').replace(/\/$/, '');
  const entries = files.map((file) => `  <sitemap><loc>${escapeText(`${base}/${file}`)}</loc></sitemap>`);
  return ['<?xml version="1.0" encoding="UTF-8"?>', '<sitemapindex>', ...entries, '</sitemapindex>', ''].join('\n');
}

export function robotsTxt(options = {}) {
  const base = String(options.base ?? 'https://example.invalid').replace(/\/$/, '');
  return ['user-agent: *', options.disallow ? `disallow: ${options.disallow}` : 'disallow:', `sitemap: ${base}/sitemap.xml`, ''].join('\n');
}

export function documentCount(xml) {
  return (String(xml).match(/<url>/g) ?? []).length;
}
