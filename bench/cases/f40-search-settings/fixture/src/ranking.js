export function scoreDocument(document, query) {
  const title = document.title.toLocaleLowerCase('en');
  const body = document.body.toLocaleLowerCase('en');
  let score = 0;
  for (const term of query.terms) {
    if (title.includes(term)) {
      score += 3;
    }
    if (body.includes(term)) {
      score += 1;
    }
  }
  return score;
}

export function rankDocuments(documents, query, sort) {
  const rows = documents.map(document => ({
    document,
    score: scoreDocument(document, query),
  }));
  rows.sort((a, b) => {
    if (sort === 'relevance' && a.score !== b.score) {
      return b.score - a.score;
    }
    const title = a.document.title.localeCompare(b.document.title, 'en');
    return title || a.document.id.localeCompare(b.document.id, 'en');
  });
  return rows;
}

export function paginate(rows, page, pageSize) {
  if (!Number.isSafeInteger(page) || page < 1) {
    throw new RangeError('page must be a positive integer');
  }
  const start = (page - 1) * pageSize;
  return {
    rows: rows.slice(start, start + pageSize),
    page,
    pageSize,
    total: rows.length,
    pages: Math.ceil(rows.length / pageSize),
    hasMore: start + pageSize < rows.length,
  };
}
