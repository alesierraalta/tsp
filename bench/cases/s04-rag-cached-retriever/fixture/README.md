# notes-retriever

Semantic retrieval for a multi-tenant notes assistant. Run `node --test`.

- `new Retriever(embed)` builds an index over an embedding model: `embed(text)` returns a vector.
- `add(owner, id, text)` stores a note.
- `search(principal, query, k = 2)` returns the ids of the `k` notes of `principal` most similar to the
  query (cosine similarity, ties by id), best first. A principal only ever sees its own notes.
- `reindex(embed)` moves the index to another embedding model; from then on the index behaves exactly as
  one built with that model from the start.

Search results are cached until the index changes.
