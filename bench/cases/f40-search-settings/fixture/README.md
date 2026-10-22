# Document search

An in-memory search component for a documentation portal, with configurable matching,
ranked results and deployment environment generation. Node ESM, no dependencies.
Run the suite with `node --test`.

`Catalog` owns documents and returns independent copies. Hidden documents are excluded
unless explicitly requested. `SearchService` interprets queries, filters by optional tag,
ranks by title/body matches and paginates results. Title matches have weight three;
body matches have weight one. Ties use title then document ID.

Interpreted queries are single-line, have no leading or trailing whitespace, and contain
at most the configured maximum number of JavaScript string characters. Control characters
are treated as whitespace. Matching is case-insensitive; an empty query lists documents.

Every setting in [docs/settings.md](docs/settings.md) can be set through the deployment
environment. `deploy/environment.mjs` renders an allowlisted environment object, validates
its values and builds a service manifest; unrelated host variables are not forwarded.
See the settings document for defaults and operational usage.
