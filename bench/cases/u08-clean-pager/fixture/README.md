# keyset pager

Decodes the page request of a keyset-paginated list endpoint.

- `ClampLimit(limit)` returns the page size a request gets: a limit of zero or less becomes `DefaultLimit`,
  a limit above `MaxLimit` becomes `MaxLimit`, and anything in between is returned unchanged.
- `Encode(cursor)` writes a cursor as a token. A cursor needs a non-zero time and an id of at most `MaxID`
  bytes that matches `[A-Za-z0-9_.-]+`; anything else is refused with `ErrToken`.
- `Decode(token)` reads a token back. A token longer than `MaxToken`, a token that is not base64url, a
  payload without both parts, a time that is not RFC 3339 with nanoseconds, and an id outside the pattern
  are all refused with `ErrToken`, so a caller cannot hand the pager a token of its own making.
- `Query(bounds, request)` returns the `Page` a request asks for: the clamped limit plus the cursor the
  request carries, or `Query` with no cursor when the token is empty. A cursor that sits after `bounds` is
  refused with `ErrRange`.
- `Window(from, to)` returns the width of a range and refuses a range that does not move forward with
  `ErrRange`.

Run `go test ./...`.
