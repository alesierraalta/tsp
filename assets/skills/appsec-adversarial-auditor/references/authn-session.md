# AuthN, session, JWT, OAuth/OIDC

Oracle for every row: the probe must be REJECTED, and a valid-token control must be ACCEPTED on the same route (see the positive-control rule in `SKILL.md`). Use synthetic identities only.

| Flaw | Probe | Vulnerable if |
|---|---|---|
| JWT `alg: none` | strip signature, set `alg` to `none` | request accepted |
| JWT alg confusion | RS256 token re-signed HS256 with the public key as secret | request accepted |
| `kid` / `jku` / `x5u` injection | point at attacker-controlled key or path traversal | token verified with attacker key |
| Missing `aud` / `iss` / `exp` checks | token for another audience or issuer; expired token | accepted |
| No revocation | reuse token after logout, password change, role removal | still accepted |
| Stale authority after a role or tenant change | downgrade or remove a role, move the user to another tenant, then reuse the old session or token | old grants still enforced |
| Refresh-token rotation | replay an already-rotated refresh token | replay accepted, or the token family is not revoked |
| Delegated or service identity | exchange a user token for a service token; use an impersonation or act-as flow; run an MCP client credential | the service or agent acts with broader authority than the delegating user |
| Session fixation | pre-login session id survives login | same id after auth |
| Idle and absolute lifetime | leave a session idle past policy; keep it active past the absolute cap | still accepted |
| SSO / IdP lifecycle | remove the user from the IdP group or deprovision at the IdP | the local session keeps access |
| Reset token | reuse it; use it after password change; tamper the Host header on request | second use works, or link points at attacker host |
| OAuth `redirect_uri` | prefix/subdomain/path-suffix/`@` variants of the registered URI | code delivered to a non-registered URI |
| OAuth `state` / `nonce` | omit, reuse, or replay from another session | callback accepted |
| PKCE downgrade | drop `code_challenge` on a public client | code exchange succeeds without verifier |
| Enumeration / MFA | compare response body and timing for known vs unknown user; skip or brute the MFA step | distinguishable or bypassable |

Traceability: cite ASVS 5.0 requirement ids (https://github.com/OWASP/ASVS , https://asvs.dev/) in the evidence row; do not restate the checklist.

A missing check in code is a lead. The violation is an accepted request after the lifecycle event, with the authorized control succeeding in the same run. Deployment facts still need validation: IdP session and token TTLs, refresh and revocation settings, cookie flags and domain, gateway caching of authenticated responses, and whether logout reaches every relying party. Record an unvalidated item as `blocked`.
