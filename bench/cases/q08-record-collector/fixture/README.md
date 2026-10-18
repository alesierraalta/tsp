# Record collector

A synchronous record lookup and cleanup library. Run `node --test`.

`new Collector({lookup, release, log, limit})` accepts ordinary adapters. `lookup(keys)` returns a parallel array of values for the supplied keys. `release(key)` returns a success boolean. `log.read()` returns the retained failure entries and `log.write(entries)` replaces them. Adapters do not throw. Defaults echo lookup keys, successfully release, and discard log writes. Limit defaults to two; finite numeric limits are floored and clamped to zero.

`records(keys)` accepts an array of arbitrary string keys, including empty and reserved property names. It returns one value per input in the same order, preserving duplicates. Values are cached per instance. Every call performs at most one lookup containing only distinct uncached keys; fully cached and empty requests make no lookup.

`collect(keys)` attempts each supplied release, including duplicates, in order. It retains only failed keys, appending them to the existing log and retaining the latest `limit` entries overall. Zero capacity retains nothing; an empty collection still enforces the configured capacity.
