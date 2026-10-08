package decker

import (
	"math"
	"testing"
)

// TestPlaceholderBoxCentersItsText pins the label's ink to the middle of the
// box however many lines what wraps to; it used to be placed for two lines,
// so a long what sat low.
func TestPlaceholderBoxCentersItsText(t *testing.T) {
	c := Ctx{W: 200, H: 60, Theme: testTheme}
	x, y, w, h := 20.0, 10.0, 160.0, 100.0
	lineH := float64(c.SmallText(testTheme.Body)) * DefaultLeading
	for _, what := range []string{
		"short",
		"a much longer description of the screenshot that belongs in this box, long enough to wrap onto four or five lines",
	} {
		p := NewPixels(c.W, 2*c.H, testTheme.Background)
		PlaceholderBox(c, p, x, y, w, h, what)
		// The ink's rows, inside the dashed frame.
		top, bottom := -1, -1
		for py := int(y) + 3; py < int(y+h)-3; py++ {
			for px := int(x) + 3; px < int(x+w)-3; px++ {
				if p.At(px, py) != testTheme.Background {
					if top < 0 {
						top = py
					}
					bottom = py
					break
				}
			}
		}
		if top < 0 {
			t.Fatalf("%q: no text drawn", what)
		}
		if off := float64(top+bottom+1)/2 - (y + h/2); math.Abs(off) > 0.35*lineH {
			t.Errorf("%q: ink spans rows %d-%d, its middle %.1fpx from the box's", what, top, bottom, off)
		}
	}
}
