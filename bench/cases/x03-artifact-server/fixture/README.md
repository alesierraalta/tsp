# artifactd

artifactd is a CI artifact server: a pipeline stages what it produced, the server records it, packs it
into the archive a release needs, and tells the project's webhooks that an artifact arrived. It serves
the CI console over HTTP and it is a single process over a local tree with the standard library only.
Run `go test ./...`.

Configuration comes from the process environment (`ARTIFACTS_ADDR`, `ARTIFACTS_ROOT`,
`ARTIFACTS_PROJECTS`, `ARTIFACTS_ALLOWLIST`, `ARTIFACTS_TOKEN_SECRET`, `ARTIFACTS_SESSION_TTL`,
`ARTIFACTS_MAX_BODY`, `ARTIFACTS_MAX_ARTIFACT`, `ARTIFACTS_MAX_ARTIFACTS`, `ARTIFACTS_RETENTION_DAYS`,
`ARTIFACTS_KEEP_LAST`, `ARTIFACTS_TOOLS`) and is validated before the listener starts. The root must be
absolute and must not be the filesystem root, there must be at least one project, no project or tool may
be listed twice, and the token secret must be at least sixteen bytes. See
[docs/operations.md](docs/operations.md) for what an operator runs.

## Contract

- **Every outbound request is decided on its parsed URL.** A delivery endpoint is parsed by
  `netguard`, and it is called only when the scheme is `https`, the URL carries no userinfo, the host is
  on the deployment's allowlist, the port is the delivery port and the host is not a private, loopback,
  link-local or metadata address — checked on the address the name resolves to, not on the text the
  subscription stores. `https://169.254.169.254/…` is the metadata service however it was typed, and
  every path that sends a delivery — the fan-out of a broadcast, a re-delivery of one subscription and
  a retry — makes that decision before anything is signed or sent.
- **A tool runs with an argument list, never a shell.** The packaging tool is named from a fixed
  allowlist and started with `exec`, so an artifact name is one argument of the process: a name may
  carry a semicolon, a space or a quote because a filesystem accepts those, and the tool packs the file
  by that name. No stage builds a command string from a value a sender wrote.
- **Every path resolves inside the root.** The tree under `ARTIFACTS_ROOT` is the only place a name can
  point: a blob, a staging directory, a pack and a staging file are all resolved through the tree's own
  reader, which refuses an absolute name, a NUL byte, a name that walks upwards and a name whose
  symbolic link leads outside the root.
- **A name is validated before it is stored.** An artifact name carries letters, digits and punctuation,
  no path separator and no traversal; a name that fails is refused with `400` and nothing is staged.
  Every name an archive carries is reported by the pack, and the manifest beside it names the same
  entries.
- **An artifact is content-addressed.** A body is stored under the SHA-256 of its bytes, so two uploads
  of the same file cost one blob, a corrupted file is detectable from its name, and a deletion drops the
  blob only when no other record names the same digest.
- **Request sizes are capped.** A body is bounded before it is decoded and a body carrying more than one
  object is refused; an artifact above the deployment's artifact cap is refused rather than truncated;
  every run the server starts is bounded by its own deadline and keeps a bounded amount of output.
- **Authorization is exact.** A token is verified before any of its fields is read, and a scope is the
  string it is: `artifacts:readonly` grants nothing that `artifacts:read` requires, and an action the
  policy table does not carry is refused. The operator routes require an operator scope and a subject
  the deployment names as its operator.
- **A caller reads its own project.** The project of a request is the project its token names, and every
  record is resolved inside it: an identifier from another project is answered as not-found.
- **Every mutation is audited.** A mutating request appends to the trail whether it was accepted,
  refused or failed, with the reason the operator needs and the client never sees, and the trail is
  readable only with the audit scope.
- **A subscription's secret never travels.** A response never carries the secret a delivery is signed
  with, and a delivery is signed per subscription over the body it carries.

## Layout

```
cmd/artifactd          the listener: configuration, service, graceful stop
artifactd.go           the wiring: Options, Open, the clock, the handler
internal/config        environment loading, validation and the caps
internal/clock         the service's clock, UTC days and windows
internal/ids           the prefixed identifiers the service mints
internal/storage       the tree: containment, content-addressed blobs, layout, quota
internal/artifacts     the artifact and job records and the retention window
internal/packaging     the tar, zip and bundle archives and the pack manifest
internal/execx         the allowlisted tools and the argument checks
internal/notify        subscriptions, the guarded delivery, the broadcast and the signature
internal/netguard      the outbound policy: allowlist, parsed URL, private ranges
internal/fetch         the bounded HTTP client a delivery uses
internal/auth          token issue and verify, the scope policy, the principal
internal/audit         the bounded trail
internal/httpx        the segment router and the response writers
internal/httpapi      the routes, the handlers and the views
```

## Endpoints

| Method | Path | Action |
| --- | --- | --- |
| GET | `/healthz` | none |
| GET | `/api/artifacts` | `artifacts.read` |
| GET | `/api/artifacts/{id}` | `artifacts.read` |
| GET | `/api/artifacts/{id}/content` | `artifacts.read` |
| POST | `/api/artifacts` | `artifacts.write` |
| POST | `/api/artifacts/{id}/pack?kind=tar\|zip\|bundle` | `artifacts.write` |
| POST | `/api/artifacts/{id}/repackage` | `artifacts.write` |
| DELETE | `/api/artifacts/{id}` | `artifacts.write` |
| GET | `/api/jobs` | `jobs.read` |
| GET | `/api/jobs/{id}` | `jobs.read` |
| GET | `/api/subscriptions` | `webhooks.read` |
| POST | `/api/subscriptions` | `webhooks.write` |
| POST | `/api/subscriptions/{id}/deliver` | `webhooks.write` |
| POST | `/api/events/broadcast` | `webhooks.write` |
| GET | `/api/admin/console` | `admin.operate` |
| GET | `/api/admin/audit` | `audit.read` |
| POST | `/api/admin/sweep` | `admin.operate` |

A project's subscriptions are told about artifacts as they arrive. `POST /api/events/broadcast` fans one
event out to every subscription of the caller's project that asks for it, `POST
/api/subscriptions/{id}/deliver` sends one event to one subscription, and neither route invents an
endpoint: the one a subscription stores is the one that is checked and called.

## Quick start

```sh
ARTIFACTS_ROOT=/srv/artifacts \
ARTIFACTS_PROJECTS=ci,release \
ARTIFACTS_ALLOWLIST=hooks.example.com \
ARTIFACTS_TOKEN_SECRET=change-me-please-please \
go run ./cmd/artifactd
```

A pipeline uploads with `POST /api/artifacts`, packs the result with `POST /api/artifacts/{id}/pack` and
publishes when the archive exists. The tree keeps a staging directory per job, the blobs a pack reads and
the packs it wrote; a sweep removes the artifacts past the retention window and keeps the project's
newest ones whatever their age.
