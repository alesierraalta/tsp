# Baseline

`tsp bench gate --baseline bench/baseline` compares a fresh run against the aggregate recorded here.

Nothing is recorded yet, and that is deliberate: a baseline is only honest when it measured the same
corpus with the same instrument as the run being compared. Copy the `aggregate.json` of a run you trust
into this directory, with the case globs and model it used written beside it, and the scheduled `bench`
workflow starts comparing against it. Until then the workflow enforces the recall floor alone and says so.
