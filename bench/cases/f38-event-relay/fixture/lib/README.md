# Vendored message-log library

This ordinary local module supplies MessageLog, LogClient, and signal utilities.
It has no external packages. LogClient's default socket timeout is 180 milliseconds.
The constructor accepts socketTimeout for applications with different transport budgets.
read(topic, cursor, {blockMs, limit, signal}) returns records after cursor or an empty array.
The socket deadline covers the whole server request, including response processing.
A deadline rejects with code SOCKET_TIMEOUT; an AbortSignal rejects with its reason.
acknowledge(subscriber, cursor, {signal}) is signal-aware and stores the latest cursor.

MessageLog exposes append, after, size, topics and the blocking read endpoint.
Signal helpers checkSignal, delay and checkpoint share the same cancellation semantics.
