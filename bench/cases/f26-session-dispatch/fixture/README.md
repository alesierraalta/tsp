# Session dispatch

Import `Sessions` from `src/index.js`. The constructor accepts a clock returning milliseconds.
The default clock is `Date.now`. Tokens are opaque local handles, not network credentials.

- `issue(subject, ttl)` creates a session for a positive lifetime in milliseconds.
- `revoke(token)` permanently revokes a session; unknown tokens are ignored.
- `refresh(token, ttl)` issues an independent session for the same subject. The source
  session must be valid; refresh never authorizes a revoked or expired source.
- `submit(token, message)` validates the session and returns an opaque queue ticket.
- `dispatch(ticket)` records one effect and consumes the ticket on success.
- `effects()` returns detached copies of recorded effects.

A session is valid only while unrevoked and strictly before its expiration instant.
Both submission and dispatch require a valid session. Queue tickets do not extend a
session lifetime. Denied calls throw without recording an effect or consuming a ticket.
Unknown tickets throw. Revoking one token does not revoke independently issued tokens.

Run `node --test`. Node ESM; no dependencies.
