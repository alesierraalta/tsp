# Job catalog

`distinctJobs(jobs)` returns the first job for each id in input order. It reads each input id at most twice, including large catalogs.

`JobLabels(limit, render)` caches rendered labels with FIFO replacement. Use a positive integer limit; retained entries never exceed it. Repeated hits do not call render. `size` reports retained entries.
