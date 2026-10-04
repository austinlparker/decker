package decker

import (
	"testing"

	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
)

// TestFramesAreOpaque checks that the footer and a transition frame have an
// explicit background color in every cell (decktest.Slides checks the
// slides). Terminals with a translucent background make default-background
// cells see-through.
func TestFramesAreOpaque(t *testing.T) {
	w, h := 120, 36
	check := func(name, frame string, rows int) {
		cv := lipgloss.NewCanvas(w, rows)
		uv.NewStyledString(frame).Draw(cv, cv.Bounds())
		for y := range rows {
			for x := range w {
				if c := cv.CellAt(x, y); c == nil || (c.Width > 0 && c.Style.Bg == nil) {
					t.Errorf("%s: cell (%d,%d) has no background color", name, x, y)
					return
				}
			}
		}
	}
	d := testDeck()
	prev := renderSlide(d.Slides[0], Ctx{W: w, H: h, T: 5, Theme: testTheme})
	next := renderSlide(d.Slides[1], Ctx{W: w, H: h, T: 1.5, Step: 2, StepT: 1.5, Theme: testTheme})
	for _, tr := range []Transition{TransitionPush, TransitionDissolve, TransitionWipe} {
		check("transition", composeTransition(tr, prev, next, w, h, 0.5, true, testTheme), h)
	}
	m := newModel(d, 3, 0, 30, nil)
	m.w, m.h = w, h
	check("footer", opaque(m.chrome(), w, chromeHeight, testTheme), chromeHeight)
}
