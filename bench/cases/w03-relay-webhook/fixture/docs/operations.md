# Operations

## Draining the queue

`relayd -interval 30s` drains the delivery queue every thirty seconds. A job is done once an
endpoint answered with a 2xx; a job that failed stays in the queue with its attempt count and its
last error. `jobs.Retryable` answers whether another attempt could help: a refused endpoint is not
retryable, because retrying it repeats the refusal and turns one policy decision into a stream of
requests.

## Reading an allowlist

`RELAY_ALLOWLIST` is a comma-separated list of host names, not URLs. An entry carrying a scheme, a
path or a port is skipped at start-up, and a deployment whose list ends up empty does not start: an
empty allowlist would allow nothing and hide the mistake behind a wall of refusals.

The guard also refuses a host that is an address literal on a private range. That rule is why a
deployment cannot be configured to call its own cluster: the addresses a service reaches for
convenience are the addresses an attacker reaches for first.

## Staging

The staging directory holds what the last import wrote. A successful import reports the paths it
wrote, relative to the staging directory, and the process logs the totals. A refused entry stops the
import at that entry: the entries applied before it stay, so an operator can see where the stream
went wrong instead of guessing from an empty directory.

## Records

The record store is in memory and the totals are integer cents. `store.Split` divides a total into
shares that add back up: a billing export that showed one cent fewer than the total would be a
reconciliation ticket every month, so the remainder is handed to the first shares rather than
dropped.

## Signatures

`X-Relay-Signature` carries `v1=<hex hmac-sha256>`, computed with the subscription's own secret over
the exact body sent. A receiver that rejects a signature can call `notify.Analyse` to learn whether
the version prefix, the secret or the body is the problem, without printing the secret.
