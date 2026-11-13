# relay

relay takes a tar stream of attachments, thumbnails and metered records, stages the files, stores
the records, and delivers the events its subscriptions ask for. It is a single process with an
in-memory record store and the standard library only; run `go test ./...`.

Configuration comes from the environment (`RELAY_ADDR`, `RELAY_STAGING`, `RELAY_ALLOWLIST`,
`RELAY_TOKEN_SECRET`) and is validated before anything is opened: the staging directory must be
absolute and not the filesystem root, the outbound allowlist must not be empty, and the token secret
must be at least 16 bytes.

## Contract

- **Only an allowed endpoint is called.** Every outbound request goes through `netguard`, which
  parses the endpoint and refuses it unless the scheme is `https`, the URL carries no userinfo, the
  parsed host is on the deployment's allowlist, the host is not a private, loopback, link-local or
  metadata address, and the port is 443. The decision is made on the parsed URL, because that is
  what the request is sent to: `https://hooks.example.com@169.254.169.254/` parses to the host
  `169.254.169.254`, whatever its text begins with. A refused endpoint is reported as
  `netguard.ErrBlocked`, never retried, and never signed.
- **A file is written only under the staging directory.** Every entry of a stream is written through
  `archive.SafeJoin`, which refuses an absolute name, a name with a NUL byte and a name that walks
  upwards, and then proves the joined path is inside the root. The check runs before any directory
  is created.
- **A stream is bounded before it is read.** At most `MaxEntries` entries, `MaxEntry` bytes each and
  `MaxTotal` bytes in total; a stream above the limits is refused, not truncated.
- **A record is validated before it is stored.** A record needs an id, an account, a kind and a
  non-negative amount; a record file carries exactly one record; a batch above `MaxBatch` is
  refused.
- **Money is integer cents.** `Split` divides a total into shares whose sum is exactly the total.
- **A signature is per subscription.** A delivery is signed with the subscription's own secret, and
  a signature covers the body it travels with.

## Layout

```
cmd/relayd            the process: configuration, one import, the queue ticker
relay.go              the wiring: guard, notifier, queue, importer, store
internal/netguard     the outbound policy: allowlist, URL parsing, private ranges
internal/fetch        the bounded HTTP client, for callers that are not deliveries
internal/notify       subscriptions, the delivery loop, the signature
internal/archive      the tar reader and the staging path rule
internal/importer     the pipeline: classify an entry, stage it or store it
internal/wire         the record decoder and its limits
internal/store        the record store, the totals and the split
internal/jobs         the delivery queue and its schedule
internal/config       environment loading and validation
```

## Entry kinds

| Directory | Written to | Rule |
| --- | --- | --- |
| `attachments/` | the staging directory | `archive.SafeJoin` |
| `thumbnails/` | the staging directory | `archive.SafeJoin` |
| `records/` | the record store | decoded by `wire`, validated, summed in cents |

An entry under any other directory stops the import with the entry's name: a stream cannot put a
file where the pipeline has no rule for it. See [docs/operations.md](docs/operations.md).
