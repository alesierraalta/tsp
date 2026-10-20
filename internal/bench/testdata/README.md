# Runner captures

Unedited stdout from real runs (one passing, one failing, one ignored test):

- `jest-29.7.0.json`: Jest 29.7.0, `jest --runInBand --json`.
- `vitest-2.1.9.json`: Vitest 2.1.9, `vitest run --globals --maxWorkers=1 --minWorkers=1 --reporter=json`.
- `cargo-1.96.1.txt`: Cargo 1.96.1 (356927216 2026-06-26), `cargo test`.

JavaScript runners used Node v24.14.1. Both npm packages were installed with exact
versions and lifecycle scripts disabled, only in a disposable project. The JSON
reports retain the original temporary paths, timestamps, and failure stacks.
