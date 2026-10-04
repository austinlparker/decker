package decker

import (
	"slices"
	"sync"
)

// sizedPool keeps up to max idle values, so big per-frame buffers are reused
// instead of reallocated.
type sizedPool[T any] struct {
	mu   sync.Mutex
	free []*T
	max  int
}

// get removes and returns an idle value for which match is true, or nil.
func (p *sizedPool[T]) get(match func(*T) bool) *T {
	p.mu.Lock()
	defer p.mu.Unlock()
	for i, v := range p.free {
		if match(v) {
			p.free = slices.Delete(p.free, i, i+1)
			return v
		}
	}
	return nil
}

// put keeps v for reuse unless the pool is full.
func (p *sizedPool[T]) put(v *T) {
	p.mu.Lock()
	if len(p.free) < p.max {
		p.free = append(p.free, v)
	}
	p.mu.Unlock()
}
