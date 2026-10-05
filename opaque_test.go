package decker

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
)

// TestFramesAreOpaque checks that the footer and a transition frame have an
// explicit background color in every cell. Terminals with a translucent
// background make default-background cells see-through.
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
	for tr := range transitions {
		sc := mixSlides(tr, d.Slides[0], d.Slides[1], 0, 2, w, h, 0.5, true)
		check("transition", sc.Render(), h)
	}
	m := newModel(d, 3, 0, 30, &devState{})
	m.w, m.h = w, h
	lines := strings.Split(view(m), "\n")
	check("footer", lines[len(lines)-chromeHeight], chromeHeight)
}
