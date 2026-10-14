# Project vault

An in-memory editorial workspace for project documents, deferred edits and recipient links.
Run `node --test`. No installation is required. Import Workspace from src/workspace.js and
Links from src/links.js. Owners manage membership; editors create and update documents;
readers can inspect project content. Lists are stable by identifier and use bounded pages.

Deferred edits are validated when prepared, but execution must be authorized by the current
project membership. Failed execution leaves the document unchanged. Cancellation discards
an unused job. Audit events describe completed mutations, not attempted operations.

See docs/sharing.md for link lifecycle guarantees. Store is an internal persistence adapter;
its methods deliberately do not authenticate callers. Policy snapshots are immutable views
of membership at the instant they are obtained, not live authorization capabilities.
