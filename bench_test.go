package decker

import (
	"io"
	"testing"
	"time"
)

// BenchmarkLive measures the live deck's whole frame at the presenting
// size: drawing it, then comparing it with the last frame and encoding the
// changes for the terminal. "transition" is mid-way through a Push, where
// every cell changes every frame; "morph" is mid-way through a Morph with
// five elements gliding or fading. A talk's per-slide drawing cost is
// measured by its own BenchmarkFrames (decktest.Frames).
func BenchmarkLive(b *testing.B) {
	for _, tc := range []struct {
		name  string
		deck  *Deck
		slide int
		trans Transition
	}{
		{"title", testDeck(), 0, TransitionNone},
		{"transition", testDeck(), 1, TransitionPush},
		{"morph", &Deck{Name: "morph", Theme: testTheme, Slides: morphSlides()}, 1, TransitionMorph},
	} {
		d := tc.deck
		b.Run(tc.name, func(b *testing.B) {
			m := newModel(d, tc.slide, 0, 60, nil)
			m.w, m.h = 682, 171
			tw := &termWriter{out: io.Discard}
			start := m.now
			for n := range b.N {
				m.now = start.Add(time.Duration(n) * time.Second / 60)
				if tc.trans != TransitionNone {
					// Stay mid-transition: restart it every frame.
					m.transFrom = drawSlide(d.Slides[0], m.ctx(m.h))
					m.trans, m.transFwd, m.transDur = tc.trans, true, TransitionDuration
					m.transStart = m.now.Add(-time.Duration(TransitionDuration / 2 * float64(time.Second)))
				}
				tw.write(m.frame(), "", false)
			}
		})
	}
}
