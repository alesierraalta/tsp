# Booking protocol

A hold expires exclusively at its elapsed-clock deadline. Wall-clock corrections never
shorten or extend its lifetime. Confirmation requires an active hold and makes it permanent;
cancellation releases capacity. Expired holds are omitted from occupancy and agendas.
Capacity is enforced per service and start instant. Disabled services reject new holds but
do not invalidate existing ones. The default hold duration is sixty seconds.

Clock.advance advances both domains; Clock.adjustWall changes only calendar time. Clock.measure
reports elapsed duration. All units are integer milliseconds, all calendar output is UTC.
