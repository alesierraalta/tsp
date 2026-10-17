# Resource desk

A synchronous resource desk. Run `node --test`.

`new Desk({release, schedule, quota})` accepts `release(tenant)` to dispose a resource and `schedule(callback)` to arrange its eventual disposal. The default adapters do nothing. Adapters do not throw; schedule may retain callbacks or invoke them immediately. Quota defaults to two; finite numeric quotas are floored and clamped to zero.

`acquire(tenant)` returns a handle with `close()`, or null when that tenant has reached its quota. Tenant names are arbitrary strings, including empty or reserved property names. Each tenant's capacity is independent. Either closing the handle or executing its scheduled callback disposes the resource, restores capacity, and calls release exactly once, regardless of how often either action is repeated or their order. Rejected acquisitions schedule nothing.
