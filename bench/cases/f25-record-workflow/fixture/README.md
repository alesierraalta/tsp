# Record workflow

An in-memory library for owner-managed titles. Import `Records` from `src/index.js`.

- `create(id, owner, title)` creates a unique record; callers supply unique ids.
- `read(id)` returns a detached snapshot, or null for an absent record.
- `transfer(actor, id, owner)` changes ownership. Only the current owner may transfer.
- `rename(actor, id, title)` changes a title and retains the previous title for undo.
- `prepareRename(actor, ids, title)` validates ownership and returns an opaque ticket.
- `applyRename(ticket)` applies the entire batch atomically and consumes its ticket on success.
- `undoRename(actor, id)` restores the last direct rename by that actor and consumes its history on success.

Every title mutation requires the initiating actor to be the current owner at the time
of mutation. Tickets and history do not grant ownership. A denied operation throws and
leaves all records unchanged. Ownership transfers preserve titles, history and pending
tickets. Unknown tickets throw. Failed operations do not consume tickets or history.

Run `node --test`. Node ESM; no dependencies.
