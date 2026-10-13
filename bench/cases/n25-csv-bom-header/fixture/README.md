# CSV importer

`parseCsv(text)` reads a header row and comma-separated data rows into records.

Header names are returned as record keys, and each record retains its column values.
Rows with missing columns are padded with empty strings.
