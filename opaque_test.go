package decker

import (
	"testing"

	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
)

// TestFramesAreOpaque checks that every cell of every slide, the footer, and
// a transition frame has an explicit background color. Terminals with a
// translucent background make default-background cells see-through.
func TestFramesAreOpaque(t *testing.T) {
	w, h := 120, 36
	check := func(name, frame string, rows int) {
		cv := lipgloss.NewCanvas(w, rows)
		uv.NewStyledString(frame).Draw(cv, cv.Bounds())
		for y := 0; y < rows; y++ {
			for x := 0; x < w; x++ {
				if c := cv.CellAt(x, y); c == nil || (c.Width > 0 && c.Style.Bg == nil) {
					t.Errorf("%s: cell (%d,%d) has no background color", name, x, y)
					return
				}
			}
		}
	}
	d := testDeck()
	slides := d.Slides
	for i, s := range slides {
		c := Ctx{W: w, H: h, T: 1.5, Step: s.steps() - 1, StepT: 1.5, Theme: testTheme}
		check(s.Title, renderSlide(s, c), h)
		if i == 1 {
			for _, tr := range []Transition{TransitionPush, TransitionDissolve, TransitionWipe} {
				prev := renderSlide(slides[0], Ctx{W: w, H: h, T: 5, Theme: testTheme})
				check("transition", composeTransition(tr, prev, renderSlide(s, c), w, h, 0.5, true, testTheme), h)
			}
		}
	}
	m := newModel(d, 3, 0, 30, nil)
	m.w, m.h = w, h
	check("footer", opaque(m.chrome(), w, chromeHeight, testTheme), chromeHeight)
}
