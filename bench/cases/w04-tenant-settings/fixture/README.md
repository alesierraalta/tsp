# tenant settings gateway

HTTP gateway for the workspace console: tenants, their members, per-tenant and per-user settings, audit
trails, and the reports the console downloads.

## Endpoints

| method | path | who |
|---|---|---|
| GET | `/health` | anyone |
| GET | `/whoami` | any valid token |
| GET | `/stats` | anyone (counts only) |
| GET | `/tenants` | platform sees every tenant, everybody else their own |
| POST | `/tenants` | platform |
| GET | `/tenants/:id` | members of that tenant |
| PUT | `/tenants/:id` | tenant admins |
| GET | `/tenants/lookup/:slug` | platform |
| GET | `/tenants/:id/members` | members |
| GET | `/tenants/:id/audit?limit=` | tenant admins |
| GET | `/tenants/:id/settings` | members |
| PUT | `/tenants/:id/settings` | members with the write right |
| PATCH | `/tenants/:id/settings/features` | tenant admins |
| POST | `/tenants/:id/settings/reset` | tenant admins |
| POST | `/tenants/:id/settings/validate` | members |
| GET | `/tenants/:id/settings/public` | members |
| GET | `/tenants/:id/users?role=&limit=&offset=&sort=&dir=` | members |
| POST | `/tenants/:id/users` | tenant admins |
| GET | `/users/:id` | self, or a tenant admin |
| PUT | `/users/:id` | self renames, admins rename others |
| PUT | `/users/:id/roles` | tenant admins, never your own roles |
| GET | `/users/:id/settings` | self, or a tenant admin |
| PUT | `/users/:id/settings` | self, or a tenant admin |
| PUT | `/users/:id/settings/bulk` | self, or a tenant admin |
| GET | `/users/:id/settings/schema` | self, or a tenant admin |
| GET | `/users/:id/sessions` | self, or a tenant admin |
| GET | `/reports`, `/reports/:name` | members |
| POST | `/reports/:name` | members with the write right |
| GET/PUT | `/reports/archive/:name` | members read, tenant admins write |
| GET | `/reports/archive/list` | any valid token |
| GET | `/reports/export/:name` | members |
| GET/PUT/DELETE | `/documents`, `/documents/:name` | members read and write, admins remove |

`POST /tenants` also accepts a partial `settings` object; `PUT /tenants/:id` accepts `name`, `slug` and
`plan`. A body that is not a JSON object, or is larger than 64 KiB, is refused with a JSON error.

## Settings

Settings are validated against a schema: `locale` (`xx` or `xx-XX`), `retention_days` (a whole number
between 1 and 3650), `digest` (`daily`, `weekly`, `never`), `features` (`audit_log`, `exports`,
`api_tokens`, all booleans), and `notifications` (`email` boolean, `webhook_url` an https URL). User
settings are `theme`, `density`, `timezone`, `digest` and `shortcuts`.

Every settings write is filtered by `mergeSettings`: only keys the schema declares are copied, nested
objects are merged key by key, and the keys `__proto__`, `constructor` and `prototype` are refused at any
depth with `400 invalid_field`. A patch that names an undeclared key is refused the same way, so a write
can add settings but never a field the console does not render.

## Files

Every file the gateway reads, writes or lists is resolved under a configured root by `resolveUnder`; a
name that is absolute, contains a path separator, or resolves outside the root is refused with
`400 unsafe_path`. Archive entries are `.json` or `.csv`; documents are capped at 1 MiB.

## Tokens

`Authorization: Bearer <token>` where the token is `<base64url payload>.<hmac sha256>`, signed with
`SETTINGS_SECRET`. The payload carries `sub`, `tenant`, `roles` and `exp`; a token more than 30 seconds
past its expiry is refused, and so is one signed with anything else.

## Running it

```sh
node src/main.js                 # PORT, SETTINGS_SECRET, DATA_ROOT, ARCHIVE_ROOT, DOCUMENT_ROOT
node --test
```
