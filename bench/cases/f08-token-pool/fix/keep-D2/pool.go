package pool

// Pool tracks available tokens for a sequential worker.
type Pool struct {
	capacity  int
	available int
}

func New(capacity int) *Pool {
	if capacity < 1 {
		panic("capacity")
	}
	return &Pool{capacity: capacity, available: capacity}
}

func (p *Pool) Reserve(n int) bool {
	if n < 1 || n > p.available {
		return false
	}
	p.available -= n
	return true
}

func (p *Pool) Refund(n int) {
	if n < 0 {
		panic("refund")
	}
	p.available += n
	if p.available > p.capacity {
		p.available = p.capacity - 1
	}
}

func (p *Pool) Available() int { return p.available }
