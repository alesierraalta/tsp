# webhook relay

Drains an outbound webhook queue: validates each target, signs the body, posts it with retries, and runs
the configured post-import hook when an import finishes. No dependencies; every module is ESM and runs on
`node --test`.

## Egress

`src/egress/rules.js` declares the rules; `src/egress/url.js` applies them:

- only `https:` is admitted unless `allowInsecure` is set;
- a host must be on the allowlist when one is configured (an entry admits the host itself and its
  subdomains, and nothing that merely ends with the same letters);
- loopback, `localhost`, the private IPv4 ranges, link-local and `::1` are refused whatever the allowlist
  says, unless `allowInternal` is set.

`assertTarget(raw, rules)` parses the URL and runs those three checks against the parsed hostname.
`src/jobs/dispatcher.js` calls it before every attempt and `Relay.enqueue` calls it before a delivery is
stored. `isInternalHost(hostname, rules)` reports whether a hostname is one of the internal ones.

## Delivery

`DeliveryStore` keeps each delivery's event, target, payload, attempts, status and next attempt. `enqueue`
refuses a second queued delivery with the same event, target, entity and topic. The dispatcher marks an
attempt before posting, treats a 4xx/5xx or a transport failure as retryable, and schedules the next
attempt with exponential backoff until `maxAttempts`. A blocked target is marked failed without a post.

## Signing

Every post carries `x-relay-signature: t=<unix>,v1=<hex hmac sha256 of "t.body">` and `x-relay-key`. Keys
are stored in `KeyStore`; `verifyMessage(body, header, secret, { toleranceSeconds })` rejects a timestamp
outside the tolerance and compares the digest in constant time. `secretLabel` only ever exposes the last
four characters.

## Import hooks

`HookRegistry` maps an event (`import.finished`, `import.failed`, `delivery.failed`,
`delivery.delivered`) to a script. `runPostImportHook(registry, payloadName)` runs the script for
`import.finished` with the payload file name as its only argument. The payload name is a plain
`[A-Za-z0-9._-]` token: `src/hooks/runner.js` validates every argument with `assertHookArgument` and
spawns the script with `node <script> <args>` and no shell, so a name that carries a space, a semicolon
or a path separator is refused instead of being interpreted.

## Running it

```sh
node src/main.js     # queues one sample delivery and prints the drain results
node --test
```
