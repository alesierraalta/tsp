# Wire import

An in-memory transport adapter for JSON-compatible row arrays and UTF-8 text supplied as a Uint8Array.

`packRows(rows)` returns a packet with a row count and a JSON payload. `unpackRows(packet)` returns all original rows in order, preserving their values. Packets are produced by this adapter; malformed or externally edited packets are outside the API contract.

`readText(bytes)` returns the text encoded by a complete valid UTF-8 byte array, preserving every character. Text can contain arbitrary Unicode labels. Input length does not change either API's guarantees. Inputs are limited only by available memory.
