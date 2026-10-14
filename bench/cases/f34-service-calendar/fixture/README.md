# Service calendar

An in-memory UTC scheduling library for monthly maintenance and short-lived booking holds.
Run `node --test`; Node ESM is the only runtime requirement. Catalog stores service capacity
and duration. Schedule advances recurring UTC appointments; Reservations manages holds and
confirmed bookings. Reports produces detached daily agendas and service summaries.

Monthly appointments retain their original anchor day. When a month lacks that day, use its
last day without changing the anchor for later appointments. Time of day is preserved.
Preview does not consume appointments. Pause prevents advancement, and resume preserves state.

Clock has two independent domains: wall time locates calendar slots; elapsed time measures
lifetimes. Consult docs/booking.md for reservations and clock adjustment semantics.
