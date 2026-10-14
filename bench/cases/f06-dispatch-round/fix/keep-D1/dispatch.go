package dispatch

type Round struct {
	state   string
	limit   int
	used    int
	pending map[string]bool
}

func NewRound(limit int) *Round {
	return &Round{state: "ready", limit: limit, pending: make(map[string]bool)}
}

func (r *Round) Start() bool {
	if r.state != "ready" {
		return false
	}
	r.state = "active"
	return true
}

func (r *Round) Dispatch(id string) bool {
	if r.state != "active" || r.used >= r.limit || r.pending[id] {
		return false
	}
	r.pending[id] = true
	r.used++
	return true
}

func (r *Round) Acknowledge(id string) bool {
	if !r.pending[id] {
		return false
	}
	delete(r.pending, id)
	if len(r.pending) == 0 {
		r.state = "completed"
	}
	return true
}

func (r *Round) Cancel() bool {
	if r.state != "active" {
		return false
	}
	r.state = "cancelled"
	return true
}

func (r *Round) Restart() bool {
	if r.state != "completed" {
		return false
	}
	r.pending, r.used = make(map[string]bool), 0
	r.state = "active"
	return true
}

func (r *Round) State() string { return r.state }
