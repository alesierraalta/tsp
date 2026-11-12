# egress guard

Decides which outbound URLs the image proxy and the webhook dispatcher may fetch.

- `new Allowlist({ hosts, allowHttp })` builds a guard. `hosts` lists the hostnames the proxy may reach;
  an empty list means "any public host". `allowHttp` also admits `http:`; without it only `https:` is
  fetched.
- `check(raw)` returns the parsed `URL` or throws `EgressBlocked`. The URL must be absolute and use an
  admitted scheme, its host must be one of the listed hosts (when a list is given), and its host must not be
  an internal address: loopback, the private IPv4 ranges, link-local and `localhost` are always refused,
  whatever the allowlist says.
- `hostOf(raw)` returns the hostname the request will reach, and `isInternal(host)` reports whether a
  hostname is one of the internal ones.
- `describe(url)` returns `host:port` for logging, and `sameOrigin(left, right)` reports whether two URLs
  share scheme, host and port.

Run `node --test`.
