package decker

import "sync"

// memo is a concurrency-safe cache for per-frame layout, which slides redo
// every frame with the same arguments. It empties itself past max entries so a
// window resize minting new keys can't grow it without bound. The zero value
// works apart from max.
type memo[K comparable, V any] struct {
	mu  sync.Mutex
	m   map[K]V
	max int
}

// get returns the value remembered for k, computing it on a miss. Callers
// that hand the value out must not let it be modified.
func (c *memo[K, V]) get(k K, compute func() V) V {
	c.mu.Lock()
	v, ok := c.m[k]
	c.mu.Unlock()
	if ok {
		return v
	}
	v = compute()
	c.mu.Lock()
	if c.m == nil {
		c.m = map[K]V{}
	} else if len(c.m) > c.max {
		clear(c.m)
	}
	c.m[k] = v
	c.mu.Unlock()
	return v
}
