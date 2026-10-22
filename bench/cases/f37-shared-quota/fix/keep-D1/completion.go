package quota

import "sharedquota/store"

// Release completes a reservation. A zero Slot is a no-op.
func (l *Limiter) Release(slot Slot) {
	if !slot.Valid() {
		return
	}
	l.store.Transaction(slot.key, func(tx *store.Tx) {
		if tx.Owns(slot.id) {
			tx.Remove(slot.id)
		}
	})
	l.stats.released()
}
