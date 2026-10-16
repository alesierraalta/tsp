# notes-index

The retrieval index behind a multi-tenant notes assistant. Run `node --test`.

- `add(owner, id, text)` stores a note. Ids are unique across all owners.
- `search(principal, query, k = 3)` returns the ids of the `k` best-matching notes **that belong to
  `principal`**, best first; a note matches when it shares at least one term with the query, and a note
  sharing more terms ranks higher (ties by id). It returns fewer than `k` ids only when `principal` owns
  fewer than `k` matching notes. A principal never receives another owner's note.
- `delete(owner, id)` removes the owner's note: from then on no search returns it. It returns `false`
  and changes nothing when the note does not exist or belongs to someone else.

Terms are the lowercase words of the text.
