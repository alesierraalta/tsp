# Image reaper

Reclaims unused images older than the retention period while preserving leased images. Persisted leases represent image references as either one ID or a list of IDs. Collection time and retention are supplied by callers; storage is in memory. Reap returns removed IDs and updates the store.
