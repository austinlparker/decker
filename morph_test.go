package decker

import (
	"math"
	"testing"
	"time"
)

// placeSlide places one solid square per key, at x in pixels.
func placeSlide(at map[string]float64, col RGB) Slide {
	return Slide{Title: "place", View: func(c Ctx, sc *Scene) {
		for _, k := range []string{"a", "b", "k"} {
			if x, ok := at[k]; ok {
				sc.Place(k, Rect{x, 10, 10, 10}, func(p *Pixels, r Rect) { p.Rect(r.X, r.Y, r.W, r.H, col, 1) })
			}
		}
	}}
}

func morphAt(from, to Slide, p float64) *Scene {
	c := Ctx{W: 100, H: 20, T: Settled, StepT: Settled, Theme: testTheme}
	a, b := drawSlide(from, c), drawSlide(to, c)
	defer a.Release()
	mixTransition(TransitionMorph, a, b, p, true, testTheme)
	return b
}

func TestMorphGlides(t *testing.T) {
	col := testTheme.Accent
	sc := morphAt(placeSlide(map[string]float64{"k": 10}, col), placeSlide(map[string]float64{"k": 70}, col), 0.5)
	defer sc.Release()
	bg := testTheme.Background
	for _, c := range []struct {
		x    int
		want RGB
	}{{45, col}, {15, bg}, {75, bg}} {
		if got := sc.Px.At(c.x, 15); got != c.want {
			t.Errorf("half-way, pixel at x=%d is %v, want %v", c.x, got, c.want)
		}
	}
}

func TestMorphFades(t *testing.T) {
	col := testTheme.Accent
	sc := morphAt(placeSlide(map[string]float64{"a": 10}, col), placeSlide(map[string]float64{"b": 70}, col), 0.5)
	defer sc.Release()
	want := Mix(testTheme.Background, col, 0.5)
	for _, x := range []int{15, 75} {
		if got := sc.Px.At(x, 15); math.Abs(float64(got.R-want.R)) > 0.01 || math.Abs(float64(got.G-want.G)) > 0.01 {
			t.Errorf("half-way, an element on one side only at x=%d is %v, want %v", x, got, want)
		}
	}
}

// TestMorphEnds checks a morph starts on the old slide and ends on the new
// one, elements and all, and stays put when the frame sizes differ.
func TestMorphEnds(t *testing.T) {
	ms := morphSlides()
	for p, s := range map[float64]Slide{0: ms[0], 1: ms[1]} {
		c := Ctx{W: 90, H: 30, T: Settled, StepT: Settled, Theme: testTheme}
		want := renderSlideGrid(s, c)
		a, b := drawSlide(ms[0], c), drawSlide(ms[1], c)
		mixTransition(TransitionMorph, a, b, p, true, testTheme)
		got := b.toGrid()
		if got.String() != want.String() {
			t.Errorf("morph at p=%v isn't the %s", p, s.Title)
		}
		for _, sc := range []*Scene{a, b} {
			sc.Release()
		}
		got.release()
		want.release()
	}
}

func TestMorphElementPanics(t *testing.T) {
	bad := Slide{Title: "bad", View: func(c Ctx, sc *Scene) {
		sc.Place("k", Rect{0, 0, 5, 5}, func(*Pixels, Rect) { panic("in an element") })
	}}
	want := renderSlideGrid(bad, Ctx{W: 100, H: 20, T: Settled, StepT: Settled, Theme: testTheme})
	defer want.release()
	sc := morphAt(placeSlide(map[string]float64{"k": 10}, testTheme.Accent), bad, 0.5)
	got := sc.toGrid()
	defer got.release()
	if got.String() != want.String() {
		t.Error("a panicking element mid-morph doesn't show the panic as the slide would")
	}
}

func TestPlaceOffscreen(t *testing.T) {
	sc := NewScene(4, 2, testTheme)
	sc.Place("", Rect{0, 0, 4, 4}, func(p *Pixels, r Rect) { p.Rect(r.X, r.Y, r.W, r.H, testTheme.Accent, 1) })
	g := parseGrid(sc.Render(), 4, 2, testTheme)
	defer g.release()
	if got, want := g.at(0, 0).bg, testTheme.Accent.q(); got != want {
		t.Errorf("Render didn't draw the placed element: %v, want %v", got, want)
	}
}

// TestTransitionTime checks a slide's TransitionTime sets how long the live
// deck mixes, and how long video holds its first step.
func TestTransitionTime(t *testing.T) {
	ms := morphSlides()
	ms[1].TransitionTime = 1.2
	d := &Deck{Name: "slow", Theme: testTheme, Slides: ms}
	m := testModel(d, 0, 0, 80, 24, Settled, nil)
	m.goTo(1, 0, true)
	m.advance(m.now.Add(time.Second))
	if m.transFrom == nil {
		t.Fatal("the transition ended before its TransitionTime")
	}
	m.advance(m.now.Add(300 * time.Millisecond))
	if m.transFrom != nil {
		t.Error("the transition outlasted its TransitionTime")
	}
	if got := videoTiming(ms[1], 0, 2); got != 3.2 {
		t.Errorf("video holds the first step %vs, want 3.2", got)
	}
	if got := videoTiming(ms[0], 0, 2); got != 2+TransitionDuration {
		t.Errorf("with no TransitionTime, video holds the first step %vs", got)
	}
}

// TestModelKeepsElements checks the live deck hands a morph the outgoing
// slide with its elements still to draw, except mid-transition, when the
// frame on screen is already a mix.
func TestModelKeepsElements(t *testing.T) {
	d := &Deck{Name: "morph", Theme: testTheme, Slides: morphSlides()}
	m := testModel(d, 0, 0, 80, 24, Settled, nil)
	m.goTo(1, 0, true)
	if m.transFrom.finished || len(m.transFrom.placed) == 0 {
		t.Fatal("the outgoing frame lost its elements")
	}
	m.now = m.now.Add(100 * time.Millisecond)
	m.goTo(0, 0, false)
	if !m.transFrom.finished {
		t.Error("a frame captured mid-transition should be finished")
	}
}
