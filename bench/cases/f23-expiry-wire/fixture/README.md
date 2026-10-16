# Expiry wire adapter

`decode(payload)` returns an expiry record with `expiresAt` measured in Unix milliseconds.
`encode(record, version = 2)` returns JSON for the requested wire version.
Version 1 stores `expires` in Unix seconds; version 2 stores it in Unix milliseconds.
The adapter interoperates with independently produced payloads of either version.
Records supplied for version 1 have whole-second precision. Unsupported versions throw RangeError.
No clock is read and expiry instants are not rounded or clamped.

Run `node --test`.
