import { slugify } from '../util/text.js';

export function menuItem(input = {}) {
  const label = String(input.label ?? input.title ?? 'Menu item');
  return {
    id: String(input.id ?? slugify(label)),
    label,
    href: String(input.href ?? `/${slugify(label)}`),
    order: Number.isFinite(Number(input.order)) ? Number(input.order) : 100,
    children: (input.children ?? []).map((child) => menuItem(child)),
  };
}

export function sortMenu(items) {
  return [...items]
    .sort((left, right) => left.order - right.order || (left.label < right.label ? -1 : 1))
    .map((item) => ({ ...item, children: sortMenu(item.children ?? []) }));
}

export function flattenMenu(items, depth = 0, out = []) {
  for (const item of items) {
    out.push({ ...item, depth });
    flattenMenu(item.children ?? [], depth + 1, out);
  }
  return out;
}

export function menuDepth(items) {
  if (items.length === 0) return 0;
  return 1 + Math.max(...items.map((item) => menuDepth(item.children ?? [])));
}

export function findMenuItem(items, id) {
  for (const item of items) {
    if (item.id === id) return item;
    const found = findMenuItem(item.children ?? [], id);
    if (found) return found;
  }
  return undefined;
}

export function removeMenuItem(items, id) {
  return items
    .filter((item) => item.id !== id)
    .map((item) => ({ ...item, children: removeMenuItem(item.children ?? [], id) }));
}

export function insertMenuItem(items, parentId, item) {
  if (parentId === undefined) return sortMenu([...items, menuItem(item)]);
  return items.map((current) =>
    current.id === parentId
      ? { ...current, children: sortMenu([...(current.children ?? []), menuItem(item)]) }
      : { ...current, children: insertMenuItem(current.children ?? [], parentId, item) },
  );
}

export function menuFromEntries(entries, options = {}) {
  const items = entries
    .filter((entry) => entry.status === 'published')
    .map((entry) =>
      menuItem({
        id: entry.id,
        label: entry.values?.title ?? entry.slug,
        href: `${options.prefix ?? ''}/${entry.slug}`,
        order: Number(entry.values?.menu_order ?? 100),
      }),
    );
  return sortMenu(items);
}

export function breadcrumbs(items, id) {
  const walk = (nodes, trail) => {
    for (const node of nodes) {
      const next = [...trail, node];
      if (node.id === id) return next;
      const found = walk(node.children ?? [], next);
      if (found) return found;
    }
    return undefined;
  };
  return (walk(items, []) ?? []).map((node) => ({ label: node.label, href: node.href }));
}

export function navigationSummary(items) {
  const flat = flattenMenu(items);
  return {
    items: flat.length,
    roots: items.length,
    depth: menuDepth(items),
    links: flat.filter((item) => item.children.length === 0).length,
  };
}
