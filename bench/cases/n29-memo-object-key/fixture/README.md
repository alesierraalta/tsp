# memo-cache

Memoizes a one-argument lookup function for repeated calls.

- Repeating the same input reuses its cached result; different inputs never share a cache entry.
- The cache uses object identity for object arguments and value equality for primitive arguments.
- Cached results may be up to 60 seconds stale; each entry expires after 60 seconds.
