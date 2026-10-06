package decker

import "testing"

// parseGrid draws a styled string into a fresh w×h grid on the theme's
// background.
func parseGrid(s string, w, h int, t *Theme) *grid {
	g := blankGrid(w, h, t.Background)
	g.draw(0, 0, s, false, t.Text)
	return g
}

// mixSlides draws from (settled, at fstep) and to (0.4s in, at tstep) at w×h
// cells and mixes them as the deck does at progress p. The caller releases
// the scene.
func mixSlides(kind Transition, from, to Slide, fstep, tstep, w, h int, p float64, forward bool) *Scene {
	a := drawSlide(from, Ctx{W: w, H: h, T: Settled, Step: fstep, StepT: Settled, Theme: testTheme})
	defer a.Release()
	b := drawSlide(to, Ctx{W: w, H: h, T: 0.4, Step: tstep, StepT: 0.4, Theme: testTheme})
	mixTransition(kind, a, b, p, forward, testTheme)
	return b
}

// TestTransitionEnds checks every transition starts on the old frame and ends
// on the new one, in both layers and both directions.
func TestTransitionEnds(t *testing.T) {
	from, to := wideSlide(), testDeck().Slides[2]
	frame := func(s Slide, step int, settled bool) string {
		ts := 0.4
		if settled {
			ts = Settled
		}
		g := renderSlideGrid(s, Ctx{W: 90, H: 30, T: ts, Step: step, StepT: ts, Theme: testTheme})
		defer g.release()
		return g.String()
	}
	old, new := frame(from, 1, true), frame(to, 2, false)
	for kind := range transitions {
		k := Transition{kind: kind}
		for _, fwd := range []bool{true, false} {
			for p, want := range map[float64]string{0: old, 1: new} {
				sc := mixSlides(k, from, to, 1, 2, 90, 30, p, fwd)
				g := sc.toGrid()
				if got := g.String(); got != want {
					t.Errorf("transition %v forward=%v at p=%v isn't the %s frame", k, fwd, p, map[bool]string{true: "old", false: "new"}[p == 0])
				}
				g.release()
				sc.Release()
			}
		}
	}
}

// TestTransitionSizeMismatch checks frames of different sizes cut.
func TestTransitionSizeMismatch(t *testing.T) {
	a, b := NewScene(10, 5, testTheme), NewScene(12, 5, testTheme)
	a.Px.Fill(testTheme.Accent)
	mixTransition(TransitionPush, a, b, 0.5, true, testTheme)
	if b.Px.At(0, 0) != testTheme.Background {
		t.Error("frames of different sizes mixed")
	}
}
