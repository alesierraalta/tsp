package quota

import "sharedquota/store"

// Acquire reserves one slot or returns ErrFull without retaining capacity.
func (l *Limiter) Acquire(tenant string) (Slot, error) {
	key, err := tenantKey(l.config.Namespace, tenant)
	if err != nil {
		return Slot{}, err
	}
	var slot Slot
	l.store.Transaction(key, func(tx *store.Tx) {
		id, count := tx.Increment(l.config.Lease)
		if count == 1 {
			tx.ExpireAfter(l.config.Lease)
		}
		if count > l.config.Limit {
			tx.Remove(id)
			return
		}
		slot = Slot{key: key, id: id, deadline: tx.Now().Add(l.config.Lease)}
	})
	if !slot.Valid() {
		l.stats.rejected()
		return Slot{}, ErrFull
	}
	l.stats.accepted()
	return slot, nil
}
