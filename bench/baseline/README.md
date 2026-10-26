# Baseline

`tsp bench gate --baseline bench/baseline` compares a fresh run against the aggregate recorded here.

A baseline is only honest when it measured the same corpus with the same instrument as the run being
compared: the same case globs, the same runner, the same model. Copy the `aggregate.json` of a run you
trust into this directory, with that instrument written beside it here, and the scheduled `bench` workflow
starts comparing against it. Until then the workflow enforces the recall floor alone and says so.

The instrument the workflow uses is pi, the same agent this project runs locally, so no vendor API key is
needed: the workflow restores a pi home from the `TSP_PI_HOME_B64` secret (or from a self-hosted runner
that is already logged in) and prints a notice instead of a green run when neither exists.
