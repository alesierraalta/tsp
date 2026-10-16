# Tabular import

`parseTable(text)` reads tab-separated records with LF or CRLF line endings.
It preserves every field, including empty fields, in its original position.
A final line ending terminates the last record without adding another record.
An empty document returns an empty array. Fields are returned without trimming.
The format does not support quoting or embedded tabs and line endings in fields.

Run the suite with `node --test`.
