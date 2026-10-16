package reservoir

type Reservoir struct {
	capacity  int
	available int
	next      int
	holds     map[int]int
}

func New(capacity int) *Reservoir {
	return &Reservoir{capacity: capacity, available: capacity, holds: make(map[int]int)}
}

func (r *Reservoir) Reserve(units int) int {
	if units <= 0 || units > r.available {
		return 0
	}
	r.next++
	r.holds[r.next] = units
	r.available -= units
	return r.next
}

func (r *Reservoir) Refund(id int) {
	if units, ok := r.holds[id]; ok {
		r.available += units
	}
}

func (r *Reservoir) Resize(capacity int) bool {
	if capacity < r.capacity-r.available {
		return false
	}
	used := r.capacity - r.available
	r.capacity = capacity
	r.available = max(0, capacity-used)
	return true
}

func (r *Reservoir) Available() int { return r.available }
func (r *Reservoir) Capacity() int  { return r.capacity }
