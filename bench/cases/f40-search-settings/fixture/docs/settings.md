# Settings and integration

Create settings with `readSettings(environment)` before constructing `SearchService`.
Without an argument the settings reader uses `process.env`. Settings are immutable;
create a new service to adopt changed settings.

| Environment name | Default | Accepted values |
| --- | --- | --- |
| SEARCH_MAX_QUERY_LENGTH | 80 | Integer from 1 through 4096 |
| SEARCH_PAGE_SIZE | 20 | Integer from 1 through 100 |
| SEARCH_SORT | relevance | relevance, title |
| SEARCH_MATCH_MODE | all | all, any, phrase |

Empty values select defaults. Invalid supplied values raise an error. All documented
settings are supported by the deployment renderer. Feed the rendered environment to
the service process; direct local environment configuration has the same semantics.

`all` requires every query term, `any` requires at least one, and `phrase` requires the
interpreted text as a contiguous substring. The maximum is a JavaScript string length
limit, not a byte or grapheme budget. Query output always satisfies the README's
whitespace and length guarantees, regardless of matching mode.

Pagination is one-based. Empty results have zero pages. Results contain excerpts of
up to 120 characters; full document bodies remain in the catalog. Presentation helpers
produce JSON or plain text and page links using the interpreted query.
