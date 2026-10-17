# Document vault

The vault retains each distinct document, reuses identifiers for identical content, and records numeric evaluation results. Unsuccessful evaluations leave the stored result unchanged.

`Vault` exports `put(content)`, `get(id)` and `await evaluate(id)`. Content is a string; identifiers are strings. `get` returns a detached `{content, score}` object or undefined. The initial score is null. `evaluate` returns true when it records a result and false otherwise.

Constructor options:

- `fingerprint(content)`: synchronous string fingerprint function for indexing, default SHA-256. Applications may supply a fingerprint implementation to match their storage conventions.
- `evaluator(content)`: async adapter returning `{status, output}`. Status zero means successful completion and output contains the numeric result. Other statuses indicate unsuccessful completion and output contains diagnostic text. The default adapter counts words.

Run `node --test`.
