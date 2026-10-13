# Log text truncator

`truncate(text, maxCharacters)` returns at most `maxCharacters` Unicode characters.
Valid input remains valid Unicode and round-trips through UTF-8.

One trailing LF (`\n`) is stripped before truncation.
