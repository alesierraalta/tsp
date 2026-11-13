# import console

Imports CSV files into in-memory datasets, renders them as HTML pages, and exports the same rows as
CSV, TSV or SQL. No dependencies; every module is ESM and runs on `node --test`.

## Datasets

`items` (`sku` key), `orders` (`order_id` key) and `customers` (`email` key). Each dataset declares its
columns and their types in `src/etl/schema.js`: `text`, `integer`, `money`, `date`, `enum`, `email`.
Every column has a maximum length; `required` columns must be present in each row.

## Import

`loadDataset(name, csvText)` reads the file, renames the header cells to canonical column names, checks
every cell against the schema, and stores the rows with money columns converted to integer cents.
Money is never kept as a binary float in a dataset: `parseCents` reads `1,234.56`, `(12.00)` and
`USD 9.99` into whole cents, and every report, invoice, estimate and manifest total is added with
`addCents`/`sumCents`, so a total is the sum of the cents the rows hold. A row that fails validation
stops the import with the row number and the columns that are wrong; duplicate keys are refused.

## Rendering

`src/render` writes the pages. Text nodes are escaped with `escapeText` (`&`, `<`, `>`); every value
placed inside an attribute is escaped with `escapeAttr`, which also escapes `"`, `'` and a backtick;
every value placed in an `href` or `src` is first passed through `escapeUrl`, which only lets `https:`,
`http:`, `mailto:`, root-relative and fragment URLs through. `htmlAttributes` and the component
renderers (`renderField`, `renderCard`, `renderTable`, `renderBars`) all build attributes from those
helpers, so a value that carries a quote cannot end the attribute it was written into.

## Export

`src/query/builder.js` builds SQL. Identifiers go through `quoteIdent` (plain `[A-Za-z_][A-Za-z0-9_]*`
only), and every value goes through `quoteLiteral`, which renders numbers bare, booleans as `TRUE` or
`FALSE`, `null` as `NULL`, and every string with single quotes doubled. The export writers in
`src/export` use those two functions for every statement they produce, including filters, so a value
that carries a quote cannot end the literal it was written into. `src/export/csv.js` quotes a cell only
when it holds the delimiter, a quote or a newline, and doubles the quotes it wraps.

## Pages

`renderPage(name, model, config)` composes the layout and one of the pages (dashboard, imports,
validation, result, report) into a complete HTML document. `renderPage` is called from `src/pipeline.js`,
which also tracks the import history and serves the exports.

## Running it

```sh
node src/main.js        # imports the sample items and prints them as CSV
node --test
```
