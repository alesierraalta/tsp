package backfill

type admission struct {
	registry   *Registry
	connection *Connection
}

func (a *admission) warm() {
	if a.connection == nil {
		a.connection = a.registry.Open()
	}
}

func (a *admission) acquire() (uint64, bool) {
	if a.connection == nil {
		return 0, true
	}
	return a.connection.Acquire()
}

func (a *admission) release(ticket uint64) {
	if ticket != 0 {
		a.connection.Release(ticket)
	}
}
