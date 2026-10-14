package worker

import "sync"

type Inventory struct {
	mu             sync.Mutex
	stock          int
	sold           int
	yieldBeforeAct func()
	capacity       int
}

func NewInventory(capacity int, yieldBeforeAct func()) *Inventory {
	return &Inventory{stock: capacity, yieldBeforeAct: yieldBeforeAct, capacity: capacity}
}

func (i *Inventory) Buy() bool {
	i.mu.Lock()
	available := i.stock > 0
	i.mu.Unlock()
	if !available {
		return false
	}
	if i.yieldBeforeAct != nil {
		i.yieldBeforeAct()
	}
	i.mu.Lock()
	i.stock--
	i.sold++
	i.mu.Unlock()
	return true
}

func (i *Inventory) Sold() int {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.sold
}

func (i *Inventory) Stock() int {
	i.mu.Lock()
	stock := i.stock
	i.mu.Unlock()
	if stock < 0 {
		return 0
	}
	return stock
}

func (i *Inventory) Refund(quantity int) {
	if quantity <= 0 {
		return
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	i.stock += quantity
	if i.stock > i.capacity {
		i.stock = i.capacity
	}
}
