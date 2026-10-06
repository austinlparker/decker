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

// BenchmarkTransitions measures each transition's mix of two 682×171 frames,
// 40% of the way. A frame is drawn for the new slide each time, since mixing
// changes it, so "cut" (no mixing) is the baseline to subtract.
func BenchmarkTransitions(b *testing.B) {
	deck := testDeck().Slides
	wide := wideSlide()
	for _, tr := range append([]Transition{TransitionNone}, transitionKinds[2:]...) {
		name := transitionNames[tr]
		if tr == TransitionNone {
			name = "cut"
		}
		b.Run(name, func(b *testing.B) {
			const w, h = 682, 171
			from := drawSlide(deck[0], Ctx{W: w, H: h, T: Settled, StepT: Settled, Theme: testTheme})
			defer from.Release()
			from.finish()
			b.ReportAllocs()
			for range b.N {
				to := drawSlide(wide, Ctx{W: w, H: h, T: 0.4, StepT: 0.4, Theme: testTheme})
				mixTransition(tr, from, to, 0.4, true, testTheme)
				to.Release()
			}
		})
	}
}
