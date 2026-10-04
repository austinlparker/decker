package decker

import (
	"io"
	"testing"
	"time"
)

// BenchmarkLive measures the live deck's whole frame at the presenting
// size: drawing it, then comparing it with the last frame and encoding the
// changes for the terminal. "transition" is mid-way through a Push, where
// every cell changes every frame. A talk's per-slide drawing cost is
// measured by its own BenchmarkFrames (decktest.Frames).
func BenchmarkLive(b *testing.B) {
	d := testDeck()
	for _, tc := range []struct {
		name  string
		slide int
		trans bool
	}{{"title", 0, false}, {"transition", 1, true}} {
		b.Run(tc.name, func(b *testing.B) {
			m := newModel(d, tc.slide, 0, 60, nil)
			m.w, m.h = 682, 171
			tw := &termWriter{out: io.Discard}
			start := m.now
			for n := 0; n < b.N; n++ {
				m.now = start.Add(time.Duration(n) * time.Second / 60)
				if tc.trans {
					// Stay mid-transition: restart it every frame.
					m.transFrom = renderSlideGrid(d.Slides[0], m.ctx(m.h))
					m.trans, m.transFwd = TransitionPush, true
					m.transStart = m.now.Add(-time.Duration(TransitionDuration / 2 * float64(time.Second)))
				}
				tw.write(m.frame(), "", false)
			}
		})
	}
}
