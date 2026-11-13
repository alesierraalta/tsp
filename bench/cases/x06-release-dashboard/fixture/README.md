# release dashboard

A deployment and release dashboard for a handful of services: releases with a changelog, environments with
their own policies, deployments that walk a stage plan with approvals and a percentage rollout, incidents that
block a promotion, a webhook relay for the events the dashboard emits, and the HTML views an operator reads.

## Running it

```sh
node src/main.js      # PORT, DASHBOARD_ROOT, MAX_BODY_BYTES, DEFAULT_TENANT, LOG_LEVEL, DASHBOARD_ALLOWED_HOSTS
node --test
node bin/report.mjs release.json --format=markdown
node bin/send.mjs https://hooks.example.com/relay --event=release.created --dry=true
```

Bodies are JSON objects of at most `MAX_BODY_BYTES`; a larger body is refused with `413 payload_too_large`.

## Tenants, keys and roles

A key belongs to one tenant and carries scopes: `releases:read`, `releases:write`, `webhooks:write` and
`admin`. A request with no key, an unknown key, a revoked key or an expired key is `401 unauthorized`; a key
without the scope a route needs is `403 forbidden`. Routes never take the tenant from the body: the tenant is
the key's tenant.

A person's role is `viewer`, `releaser`, `approver` or `owner`, and each role carries capabilities (`read`,
`deploy`, `approve`, `rollback`, `manage`). A role is what the policy book evaluates; it is never the same
thing as a key's scope, which gates the route.

## Releases

A release is a semantic version, a service, an environment and a changelog. A version that is not
`MAJOR.MINOR.PATCH` (optionally with a prerelease) is refused with `400 invalid_version`, and the same service
and version cannot be created twice for one environment (`409 release_duplicate`). A release moves through
`planned`, `staged`, `canary`, `live`, `rolled_back` and `cancelled`; the allowed transitions are the table in
`src/domain/releases.js`, and any other move is `409 invalid_transition`. `promote` moves a release from
development to staging to production and returns it to `planned` for the new environment.

`GET /releases` lists them, filtered by `service`, `environment`, `status` and `channel`; `GET
/releases/:id/notes` renders the changelog as sections (breaking, feature, fix, security, chore) and returns
the Markdown the CLI writes.

## Environments, policies and freezes

Every environment has a tier (`development` low, `staging` medium, `production` high), the number of
approvals a promotion needs and the rollout steps it uses: `[100]`, `[50, 100]` and `[1, 10, 50, 100]`.
Production is protected.

A policy says who may do what where: `minimum_role`, the approvals it requires and the release channels it
admits. `POST /admin/policies/check` answers with `allowed` and the reasons a promotion would be refused. A
policy that refuses a deployment answers `403 policy_refused`.

A change freeze is a window (`from`, `to`, an environment and a reason). While a freeze covers an
environment, a production deployment is refused with `409 change_freeze` unless the request carries
`override: true`; the window is evaluated against the deployment's own instant.

## Deployments, stages and approvals

A deployment walks `prepare → approve → rollout → verify → done`. `POST /deployments` starts one for a
release; a service already deploying to that environment is `409 deployment_active`. Each environment's stage
plan carries the approvals the `approve` stage needs and the percentages the `rollout` stage walks.
`POST /deployments/:id/advance` moves to the next stage, `.../approve` records an approval (and requests one
when none is pending), `.../refuse` fails the deployment, `.../finish` closes it and `.../rollback` marks it
rolled back. A deployment that already finished refuses further moves with `409 deployment_finished`.

## Incidents

An incident carries a severity (`sev1`–`sev4`), the service it belongs to and an owner. `sev1` and `sev2` are
blocking: an open blocking incident on a service means the service's production releases are held. An
incident is opened, assigned and resolved; resolving twice is `409 incident_closed`.

## Dashboards

`GET /` renders the dashboard (HTML when the request accepts it, JSON otherwise): the summary, the open
deployments, the latest deliveries and the open incidents. The other dashboard routes report activity by day,
the release timeline, the DORA metrics (lead time, deployment frequency, change failure rate, time to
restore), the readiness of each service and the export of every report as `csv`, `ndjson` or `json` with a
manifest that names the rows and bytes written.

## Webhooks and egress

A subscription is a stored URL plus the events it wants (`release.created`, `deployment.started`,
`deployment.finished`, `deployment.rolled_back`, `incident.opened`). The URL is checked with `assertTarget`
before it is stored: the text is parsed as a URL, the scheme must be `https`, the **hostname the URL actually
reaches** must not be an internal address (loopback, `10/8`, `172.16/12`, `192.168/16`, link-local,
`localhost`, `::1`), and the host must be on the allowlist when the operator configured one
(`DASHBOARD_ALLOWED_HOSTS`; an empty allowlist admits any public host). A refused target is `403 forbidden`
with `egress blocked: <host> …`. The stored URL is the parsed form, so the host the policy approved is the
host a delivery posts to.

A subscription restored from the operator's stored snapshot keeps the URL it was saved with, so the delivery
path checks that URL again — through `assertTarget`, on the parsed hostname — before it prepares a post.
Deliveries are recorded with their host, their event and their attempt count; a failed delivery is retried on
the schedule in `src/webhooks/retry.js` and dropped once its attempts are spent. Every delivery carries an
`x-dashboard-signature` header (`t=…,v1=…`) that a receiver verifies with the subscription's secret.

## Views and markup

Every value the dashboard renders comes from a stored record and is escaped for the context it lands in:
element text goes through `escapeText`, and **every attribute goes through `escapeAttr`** (directly or through
`attrPair`, `element`), which also escapes the double and single quote that would otherwise close the
attribute. `src/views/*.js` build the pages; no view writes an unescaped value, and `{{{…}}}`-style raw output
does not exist here.

## Storage and jobs

`MemoryStore` holds named collections keyed by id; `EventLog` records `release.created`,
`deployment.started`, `deployment.finished`, `deployment.rolled_back`, `approval.granted`,
`webhook.received` and `incident.opened` events with actor and subject; `Cache` is a TTL cache for rendered
summaries. The queue carries `notify`, `digest`, `collect` and `sweep` jobs; a scheduled job waits for its
instant, a failed job is retried until its attempts are spent, and the reaper drops the jobs and deliveries
that fell out of the retention window.
