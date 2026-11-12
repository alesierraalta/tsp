# plugin envelope

Decodes and merges the options envelope a plugin sends with each request.

- `decode(text)` parses the envelope. It must be a JSON object no larger than `MaxBytes`; an empty body, a
  body that is not JSON, and an array are refused with `EnvelopeError`.
- `applyEnvelope(text, defaults)` decodes the envelope and merges it over `defaults`, returning the merged
  options. Merging walks nested objects, and the result carries the keys the envelope names plus the defaults
  it does not override. The defaults object itself is never mutated.
- `merge(base, patch)` is the walk `applyEnvelope` uses; `isPlainObject(value)` reports whether a value is
  one of the objects a walk descends into.
- `pick(envelope, names)` returns the named options this plugin understands and ignores the rest.
- `headerMap(lines)` reads `Name: value` lines into a map keyed by the lowercased name. A name that appears
  more than once keeps every value, in the order the lines carry them; a name that appears once maps to its
  string value.

Run `node --test`.
