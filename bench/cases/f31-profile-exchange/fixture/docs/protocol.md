# Integration protocol

POST carries a complete object; PATCH carries only mutable fields. Undefined
properties are absent on the wire, whereas JSON null is a supplied value.
Empty updates are accepted and recorded like any other successful update.
The normalization module offers compactRecord for creation payloads: it omits
null and undefined values because creation defaults omitted fields to null.
trimRecord and fieldCounts are optional preparation and reporting utilities.
The core codec is also usable by adapters that construct request bodies themselves.

CSV exports include a header in the requested column order. Each data row has
exactly one cell per requested column, including empty cells for null fields.
Commas, quotes and line breaks use standard CSV quoting; null and empty text have
the same CSV representation. JSON exports retain null values and field names.
Exports apply exact filters before sorting by id and never mutate the directory.

Audit entries are immutable internally; returned events and query rows are copies.
Snapshots and audit sequences are local to one Service instance. There is no
persistence or concurrency protocol in this package.
