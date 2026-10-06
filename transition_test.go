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

// TestTransitionDirectionEnds checks every side starts on the old frame and
// ends on the new one, for the transitions that have one.
func TestTransitionDirectionEnds(t *testing.T) {
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
	for _, base := range []Transition{TransitionPush, TransitionWipe, TransitionCover, TransitionUncover, TransitionSplit} {
		for _, d := range []Direction{DirDefault, DirRight, DirLeft, DirUp, DirDown} {
			for _, fwd := range []bool{true, false} {
				for p, want := range map[float64]string{0: old, 1: new} {
					sc := mixSlides(base.From(d), from, to, 1, 2, 90, 30, p, fwd)
					g := sc.toGrid()
					if got := g.String(); got != want {
						t.Errorf("transition %v from %v forward=%v at p=%v is wrong", base, d, fwd, p)
					}
					g.release()
					sc.Release()
				}
			}
		}
	}
}

// TestTransitionDirectionDefault checks each direction's zero value plays
// from the side the transition always did, and that going back reverses it.
func TestTransitionDirectionDefault(t *testing.T) {
	from, to := wideSlide(), testDeck().Slides[2]
	frame := func(tr Transition, p float64, fwd bool) string {
		sc := mixSlides(tr, from, to, 1, 2, 90, 30, p, fwd)
		defer sc.Release()
		return sc.Render()
	}
	for _, tc := range []struct {
		zero, explicit Transition
	}{
		{TransitionPush, TransitionPush.From(DirRight)},
		{TransitionWipe, TransitionWipe.From(DirLeft)},
		{TransitionCover, TransitionCover.From(DirRight)},
		{TransitionUncover, TransitionUncover.From(DirRight)},
	} {
		for _, p := range []float64{0.2, 0.5, 0.8} {
			if a, b := frame(tc.zero, p, true), frame(tc.explicit, p, true); a != b {
				t.Errorf("%v at p=%v differs from its explicit side", tc.zero, p)
			}
			if a, b := frame(tc.zero, p, false), frame(tc.explicit.From(tc.explicit.dir.opposite()), p, true); a != b {
				t.Errorf("%v going back at p=%v isn't from the opposite side", tc.zero, p)
			}
		}
	}
	if TransitionDefault.From(DirUp).resolve() != TransitionPush.From(DirUp) {
		t.Error("a default transition lost its direction")
	}
}

// TestTransitionVerticalCells checks vertical motion moves whole cells: in a
// push between two flat colors the seam is always between two cells, never
// inside one, so no character ever straddles it.
func TestTransitionVerticalCells(t *testing.T) {
	const w, h = 40, 21
	flat := func(c RGB) *Scene {
		sc := NewScene(w, h, testTheme)
		sc.Px.Fill(c)
		return sc
	}
	red, blue := RGB{R: 255}, RGB{B: 255}
	for _, d := range []Direction{DirUp, DirDown} {
		for p := 0.05; p < 1; p += 0.05 {
			a, b := flat(red), flat(blue)
			mixTransition(TransitionPush.From(d), a, b, p, true, testTheme)
			seam := 0
			for y := 1; y < b.Px.H; y++ {
				if b.Px.At(0, y) != b.Px.At(0, y-1) {
					seam = y
				}
			}
			if seam%2 != 0 {
				t.Errorf("push from %v at p=%.2f has its seam at pixel row %d", d, p, seam)
			}
			a.Release()
			b.Release()
		}
	}
}

// TestTransitionAllocs checks mixing a frame allocates nothing once the
// pools are warm, for every transition and side.
func TestTransitionAllocs(t *testing.T) {
	const w, h = 120, 34
	from := drawSlide(wideSlide(), Ctx{W: w, H: h, T: Settled, Step: 1, StepT: Settled, Theme: testTheme})
	defer from.Release()
	to := drawSlide(testDeck().Slides[2], Ctx{W: w, H: h, T: 0.4, Step: 2, StepT: 0.4, Theme: testTheme})
	defer to.Release()
	for _, tr := range transitionKinds[2:] {
		if tr == TransitionMorph {
			continue // draws placed elements, which the scenes here have none of
		}
		mix := func() { mixTransition(tr, from, to, 0.4, true, testTheme) }
		mix()
		if n := testing.AllocsPerRun(5, mix); n != 0 {
			t.Errorf("%s allocates %v times per frame", transitionNames[tr], n)
		}
	}
}
