# Release dispatcher

A small library that queues environment-correct release artifacts and collects outstanding deliveries. Durable deliveries remain available; transient deliveries are retained for the configured interval.

`new Dispatcher(catalog, options)` accepts catalog rows with `environment`, `version`, and `digest`. Options are `clock` (a function returning milliseconds, default `Date.now`) and `retention` (milliseconds, default 60000).

`publish(id, environment, version, mode)` queues a delivery, returning false if the artifact cannot be resolved. Mode defaults to `durable`; `transient` deliveries have a retention interval. Publishing an existing ID replaces that delivery. `collect()` returns `{id, artifact}` records for outstanding deliveries.

Run `node --test`.
