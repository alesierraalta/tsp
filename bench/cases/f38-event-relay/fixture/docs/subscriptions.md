# Subscription lifecycle

Permits are acquired when iteration starts, not when an iterator is constructed.
A permit remains held while the consumer processes a frame. Iterator return and signal
cancellation both terminate a session. Acknowledgement records the last delivered cursor;
permit release is mandatory even when transport acknowledgement cannot complete.

Payloads are detached on append and read. Topics are exact names after trimming at the
relay boundary. Cursors are nonnegative safe integers. Reads return at most batchSize
records, default 32, in append order. Keep-alives preserve the cursor and do not count
as delivered events. Metrics count opened and closed sessions and emitted frames.

The in-process message-log server models a blocking endpoint: empty reads wait for the
requested block duration, then include a small response-processing delay. An event already
available at read time is returned immediately. New arrivals during a block are returned
when that block completes. No persistence or cross-process transport is provided.
