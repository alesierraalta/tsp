# markup-renderer

Renders escaped fragments for server-generated pages.

- `renderText(value)` escapes `&`, `<`, and `>` for text-node content.
- `renderLink(label, href)` keeps attribute values from terminating their quoted attribute.
- URLs in `href` are not sanitized; the caller is responsible for supplying safe URLs.
- A style value cannot add declarations.
