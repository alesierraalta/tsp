# Passenger register

The register maintains passenger profiles and their standard planning weights. Adults have a standard weight of 84 and children 35; the recorded category determines the weight. Amending a profile preserves fields not supplied in the amendment.

`Register` exports `add(id, profile)`, `get(id)`, and `amend(id, patch)`. Profiles contain `name`, `title`, `category` and `destination`. `amend` returns false for an unknown id. Reads return detached objects.

The optional `store` is a Map-compatible persistent record store shared by register instances. It accepts named profile objects (including `weight`) and positional records `[name, title, category, destination, weight]`. Both representations have the same meaning; writes use named objects. With no options the register owns a new in-memory Map.

Run `node --test`.
