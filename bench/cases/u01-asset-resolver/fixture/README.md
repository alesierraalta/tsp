# asset-resolver

Resolves static asset paths for the web tier.

`New(root, manifest)` builds a resolver for one root directory and the bundle manifest the build emits. The
root must be absolute and is cleaned before use; a root that is the filesystem root is refused. The manifest
maps a bundle name to that bundle's path relative to the root.

- `Resolve(requestPath)` returns the absolute path of a request path. Request paths arrive URL-encoded from
  the browser, so percent-encoding is decoded before the path is used. An empty request path, a path with a
  NUL byte, an absolute path, or a path that would leave the root is refused with `ErrOutside`.
- `Bundle(name)` returns the absolute path of a manifest entry, and `ErrNoBundle` for a name the manifest does
  not carry. Every path this package returns — a resolved request path and a bundle path alike — resolves
  inside the root; a manifest entry that would leave it is refused with `ErrOutside`.
- `Key(requestPath)` returns the cache key of a request path: the path relative to the root with forward
  slashes, or `""` when the request path is refused.
- `Decode(requestPath)` percent-decodes a request path and reports malformed encoding instead of passing it
  through.
- `Manifest()` lists the bundle names in sorted order.

Run `go test ./...`.
