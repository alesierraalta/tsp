# Operations

A selection contains an optional team and a positive batch size (default 100).
Only pending profiles are eligible. Blank display names fall back to the handle;
search terms are lowercased, deduplicated, and sorted. Cards retain the profile ID
and team. Repository snapshots and index reads return independent copies.

Apply claims selected profiles, increments their attempt count, builds each card,
and writes it to the index. A successful write completes the profile. A failed
write returns that profile to pending so a later job can retry it. The report
counts successful writes and records individual failures without discarding other
cards. Context cancellation stops processing and returns unfinished claims to
pending. Preview reports the same proposed cards without reserving any profiles.

Each Worker serializes its own runs. Registry connections are opened lazily and
retained for later runs. Admission lasts for the entire job, not for each card;
all outcomes release admission. A registry is shared by workers, while each worker
has its own connection. Warm establishes a connection without submitting a job.
