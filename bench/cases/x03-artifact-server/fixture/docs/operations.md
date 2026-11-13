# Operations

artifactd is one process over one tree. This page is what an operator runs and what they look at when a
delivery does not arrive.

## Start

```sh
export ARTIFACTS_ROOT=/srv/artifacts
export ARTIFACTS_PROJECTS=ci,release
export ARTIFACTS_ALLOWLIST=hooks.example.com,deploy.example.com
export ARTIFACTS_TOKEN_SECRET="$(cat /etc/artifactd/token-secret)"
exec artifactd
```

The service validates the whole configuration before it creates a directory or opens a listener. A
relative root, the filesystem root, an empty project list, a project listed twice, an allowlist entry
that carries a scheme or a path, and a secret shorter than sixteen bytes are refusals, and the message
names the variable.

## The tree

```
$ARTIFACTS_ROOT/
  blobs/<ab>/<digest>       the artifact bytes, one file per digest
  staging/<job>/<name>      what a job staged, which a pack reads
  packs/<job>/<name>.tar    an archive a pack wrote
  manifests/<job>/pack.json what the pack reported
```

Every path is resolved by `internal/storage`, which is the only code that turns a name into a path. A
body is stored under the SHA-256 of its bytes; a second upload of the same file costs nothing and a
corrupted file is detectable from its name. `GET /api/artifacts/{id}/content` reads through the same
reader, so a symbolic link that leads out of the tree is refused rather than followed.

## Upload and pack

`POST /api/artifacts` stages a body and records it. `POST /api/artifacts/{id}/pack?kind=…` builds the
archive: `tar` and `zip` are written by this process, `bundle` is compressed by the packaging tool, and
`POST /api/artifacts/{id}/repackage` rebuilds an archive a job produced. The tool is started with an
argument list and without a shell, so the artifact's name is a file name the tool packs and never a
fragment of a command.

## Webhooks

`POST /api/subscriptions` records the endpoint a project sends its events to, the events it asks for and
the secret its deliveries are signed with. Whether the endpoint may be called is decided when a delivery
runs: the URL is parsed, and it is called only when its scheme is https, it carries no userinfo, its
host is on `ARTIFACTS_ALLOWLIST` and it is not a private, loopback, link-local or metadata address. A
refused endpoint is reported as `netguard: endpoint is not allowed` and is never signed or sent.

Three paths send a delivery: the fan-out of `POST /api/events/broadcast`, a re-delivery through
`POST /api/subscriptions/{id}/deliver`, and a retry. All three make the same decision on the same parsed
URL, so an endpoint is either callable everywhere or nowhere.

A delivery is signed with the subscription's own secret: `X-Artifact-Signature` carries the HMAC-SHA-256
of the body it travels with, so a receiver that verifies it has the bytes it verifies. A response never
carries the secret.

## Retention

`POST /api/admin/sweep` removes the artifacts past `ARTIFACTS_RETENTION_DAYS`, measured from publication
(or from creation for an artifact that was never published), and keeps the project's newest
`ARTIFACTS_KEEP_LAST` whatever their age, so a sweep cannot leave a project with nothing. A blob is
dropped only when no remaining record names the same digest.

## Audit and the console

`GET /api/admin/console` prints the configuration with the signing secret replaced by a length marker,
the record counts, the allowlist, the routes the process serves and the trail's own summary.
`GET /api/admin/audit` prints the project's trail: every accepted, refused and failed mutation with the
reason the operator needs. The trail is bounded and says how many records the bound dropped.

## Tokens and caps

A token names one subject, one project and its scopes, and its lifetime may not exceed
`ARTIFACTS_SESSION_TTL`. A scope is the string it is: `artifacts:readonly` grants nothing that
`artifacts:read` requires. The operator routes require the operator scope and a subject the deployment
names as its operator. A body is bounded before it is decoded, an artifact is refused above
`ARTIFACTS_MAX_ARTIFACT`, and every tool run is bounded by its own deadline and keeps a bounded amount of
output.
