# Credit settlement

A dependency-free usage billing library. Run `node --test`.
A Session combines product registration, basket editing, preview, checkout and ledger reporting.
Rates are integer microcredits, with 1000 microcredits per settled credit. Catalog quotes retain
that precision; invoice settlement rounds the combined charge once to the nearest credit.
Posting uses unique references and checkout clears the basket only after a successful post.
See [accounting conventions](docs/accounting.md) for numeric and reporting guarantees.
