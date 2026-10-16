# Catalog pages

An in-memory catalog used by an export service. Run `go test ./...`.
Stores own record copies and maintain sorted string IDs. List returns at most the requested
limit; Next is the last delivered ID when more records remain. Passing Next as after resumes
strictly after that ID. Missing cursors resume at the first greater ID. Pages are live views,
not snapshots. See [wire conventions](docs/wire.md) for labels and index semantics.
