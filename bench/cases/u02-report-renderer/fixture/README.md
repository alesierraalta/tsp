# order-summary renderer

Renders the order summary page of the merchant dashboard: one HTML page per order, plus the JSON-LD block
search engines read.

- `renderSummary(order)` returns the page. The order title, the customer note and every line item come from
  merchant data, so they are escaped for the context they land in: element text is escaped as text and an
  attribute value is escaped as an attribute value.
- `renderRich(markup)` renders the customer note. Notes may carry the inline formatting `<b>`, `<i>` and
  `<em>`; any markup outside those three tags is content, and content is escaped.
- `attr(name, value)` renders one attribute. The name must match `[a-z][a-z0-9-]*`; the value is escaped as
  an attribute value.
- `escapeText(value)` escapes `&`, `<` and `>` for element text.
- `money(cents, currency)` formats integer cents; `truncate(text, limit)` shortens a label to `limit`
  characters and adds an ellipsis only when it actually shortened the text.
- `renderJsonLd(order)` returns the JSON-LD block; a `<` in any string is emitted as `\u003c` so data can
  never close the script element.

The suite runs with `node --test`.
