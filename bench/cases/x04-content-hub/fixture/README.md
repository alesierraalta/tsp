# content hub

A headless content hub: content types, entries with drafts and revisions, a media library, template themes,
imports/exports, a search index and a plugin host that discovers plugins from a directory and runs the hooks
they declare.

## Endpoints

| method | path | who |
|---|---|---|
| GET | `/health` | anyone |
| GET | `/whoami` | any valid token |
| GET | `/stats` | anyone |
| GET | `/content-types` | any valid token |
| GET | `/content-types/:apiId` | any valid token |
| POST/PATCH | `/content-types`, `/content-types/:apiId` | admin |
| DELETE | `/content-types/:apiId` | admin, only while no entry uses it |
| GET | `/content-types/:apiId/entries` | any valid token |
| POST | `/content-types/:apiId/validate` | editor, admin |
| GET | `/entries?type=&status=&locale=&limit=&offset=` | any valid token |
| GET | `/entries/:id`, `/entries/by-slug/:slug` | any valid token |
| POST | `/entries`, `/entries/bulk` | editor, admin |
| PUT | `/entries/:id` | editor, admin |
| DELETE | `/entries/:id` | editor, admin |
| POST | `/entries/:id/publish`, `/entries/:id/unpublish` | editor, admin |
| POST | `/entries/:id/duplicate` | editor, admin |
| GET | `/entries/:id/revisions` | any valid token |
| POST | `/entries/:id/revisions/:revision/restore` | editor, admin |
| GET | `/media`, `/media/:name`, `/media/:name/variants`, `/media/:name/metadata` | any valid token |
| PUT/DELETE | `/media/:name` | editor, admin |
| GET | `/plugins`, `/plugins/discovered`, `/plugins/assets` | any valid token |
| POST | `/plugins/load`, `/plugins/:name/transform` | admin, editor |
| POST | `/plugins/:name/enable`, `/plugins/:name/disable` | admin |
| PATCH | `/plugins/:name` | admin |
| GET/PUT | `/plugins/assets/:name` | any valid token, admin writes |
| GET | `/search?q=&type=&locale=`, `/search/suggest`, `/search/stats` | any valid token |
| POST | `/search/reindex` | editor, admin |
| GET/POST | `/exports`, `/imports`, `/imports/validate` | any valid token, editor and admin write |
| GET | `/admin/*` | any valid token, mutations for admin |

Bodies are JSON objects of at most 512 KiB; a larger body is refused with `413 payload_too_large`.

## Entries

An entry belongs to one content type, one locale and one status (`draft`, `published`, `archived`). A
content type declares its fields; `title` and `body` are the fields the article type ships with. Values are
validated against the declared fields, a field the type does not declare is refused with
`400 invalid_entry`, and an entry holds at most 256 KiB of values.

The slug is derived from the title unless the body names one; it is unique per type and locale, and a second
entry asking for a taken slug is refused with `409 duplicate_slug`. Publishing validates the entry against its
type first and moves it to `published`, which sets `published_at`; transitions between `draft`, `published`
and `archived` follow the table in `src/domain/publishing.js`, and anything else is `409 invalid_transition`.

## Files

Every file the hub reads, writes or lists is resolved under a configured root by `resolveUnder`; a name that
is absolute, contains a path separator, or resolves outside the root is refused with `400 unsafe_path`. Media
files are capped at 1 MiB and plugin assets at 512 KiB; exports are written under `EXPORT_ROOT`, media under
`MEDIA_ROOT`, templates under `TEMPLATE_ROOT`, plugin manifests and assets under `PLUGIN_ROOT`.

## Templates

Templates are `{{variable}}` documents with `{{#if}}`, `{{#each}}` and inline helpers such as
`{{upper title}}` or `{{attr href}}`. `escapeText` is applied to `{{…}}`, `{{{…}}}` emits the value raw, and
`escapeAttr` is what every attribute in `src/templates/escaping.js` goes through. A template name is read
through the loader, so it is a plain file name under the template root.

## Plugins

A plugin is a directory under `PLUGIN_ROOT` holding `plugin.json` with `name`, `version`, `hooks` and
`permissions`. Hooks run in registration order and receive the payload; a plugin may only use the
capabilities it declares, and a hook that needs a capability the manifest does not declare is refused with
`403 forbidden`.

Post-render transforms and the export pipeline run the local tools in `bin/` through `runTool`, which
validates every argument and spawns the tool with an argument vector and no shell, so a slug, a template name
or a file name reaches the tool as exactly one argument. `bin/render.mjs`, `bin/checksum.mjs` and
`bin/resize.mjs` are those tools.

## Running it

```sh
node src/main.js     # PORT, HUB_SECRET, HUB_ROOT, MEDIA_ROOT, TEMPLATE_ROOT, PLUGIN_ROOT, EXPORT_ROOT
node --test
```
