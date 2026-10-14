package workqueue

import "sync"

type cacheRow struct {
	value      string
	generation uint64
}

type Cache struct {
	mu          sync.Mutex
	rows        map[string]cacheRow
	generations map[string]uint64
}

func NewCache() *Cache {
	return &Cache{rows: make(map[string]cacheRow), generations: make(map[string]uint64)}
}

func (c *Cache) Lookup(key string) (string, uint64, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	generation := c.generations[key]
	row, ok := c.rows[key]
	return row.value, generation, ok && row.generation == generation
}

func (c *Cache) Store(key, value string, generation uint64) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.generations[key] != generation {
		return false
	}
	c.rows[key] = cacheRow{value: value, generation: generation}
	return true
}

func (c *Cache) Invalidate(key string) uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.generations[key]++
	delete(c.rows, key)
	return c.generations[key]
}

func (c *Cache) Size() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.rows)
}
