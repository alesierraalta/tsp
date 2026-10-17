# Record journal

A small library for validated record ingestion and independent pending transactions. Invalid records leave stored rows unchanged; pending transaction values remain associated with their returned handles until committed.

`new Journal(options)` accepts `columns` (ordered field names, default `['key', 'value']`) and `state` (a persisted allocator object containing numeric `next` and an `entries` Map). A restored state is an allocator state previously reached by a Journal instance. The default state is empty. Handles are unsigned 16-bit integers; `begin(value)` returns null when capacity is occupied. `commit(handle)` consumes and returns its value.

`configure(columns)` updates the input layout. `ingest(fields)` requires a key and value plus every configured field, returns whether the record was accepted, and stores the decoded object in `rows` under its first field. Extra input fields are allowed. Empty strings are valid fields.

Run `node --test`.
