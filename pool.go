package decker

import (
	"slices"
	"sync"
)

// sizedPool keeps up to max idle values so big per-frame buffers are reused.
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

// drain forgets every idle value, for a pool whose sizes changed: the old
// ones would otherwise fill it and keep the new size from ever being kept.
func (p *sizedPool[T]) drain() {
	p.mu.Lock()
	clear(p.free)
	p.free = p.free[:0]
	p.mu.Unlock()
}

// put keeps v for reuse unless the pool is full.
func (p *sizedPool[T]) put(v *T) {
	p.mu.Lock()
	if len(p.free) < p.max {
		p.free = append(p.free, v)
	}
	p.mu.Unlock()
}

// scratchPix lends pixel buffers a transition needs besides its two frames
// (a copy of one, or a row), so none is allocated per frame.
var scratchPix = sizedPool[Pixels]{max: 4}

// getScratch returns a buffer of n pixels with undefined contents; hand it
// back with putScratch.
func getScratch(n int) *Pixels {
	if p := scratchPix.get(func(p *Pixels) bool { return len(p.Pix) == n }); p != nil {
		return p
	}
	scratchPix.drain()
	return &Pixels{Pix: make([]RGB, n)}
}

func putScratch(p *Pixels) { scratchPix.put(p) }
