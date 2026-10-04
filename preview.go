package decker

// previewKey names one preview: a slide at a step in a pw×ph box, drawn for a
// dw×dh deck.
type previewKey struct{ slide, step, pw, ph, dw, dh int }

// key describes the preview of slide i at step: the largest box in the
// deck's shape that fits beside its twin and leaves room for notes. ok is
// false before the deck has reported its size, or when the box is too small.
func (p presenter) key(i, step int) (k previewKey, ok bool) {
	dw, dh := p.st.W, p.st.H
	if dw <= 0 || dh <= 0 {
		return k, false
	}
	pw := (p.inner() - presGutter - 4) / 2 // each frame adds 2 columns
	// A cell shows one pixel across and two down, like the deck's, so the
	// deck's shape in cells carries over directly.
	ph := pw * dh / dw
	if most := p.h - presHeader - footerLines - 3 - 6; ph > most { // label + frame, and 6 lines of notes
		ph = most
		pw = ph * dw / dh
	}
	return previewKey{i, step, pw, ph, dw, dh}, ph >= 5 && pw >= 20
}

// renderPreview draws s at a size it's designed for (240 cells wide, in the
// deck's shape), then shrinks it into pw×ph cells; drawing at preview size
// would lay the slide out for a tiny screen instead.
func renderPreview(s Slide, k previewKey, t *Theme) string {
	const rw = 240
	rh := max(rw*k.dh/k.dw, 20)
	g := renderSlideGrid(s, Ctx{W: rw, H: rh, T: Settled, Step: k.step, StepT: Settled, Theme: t})
	defer g.release()
	sc := NewScene(k.pw, k.ph, t)
	shrinkInto(g.pixels(), sc.Px)
	return sc.Render()
}

func shrinkInto(src, dst *Pixels) {
	for y := 0; y < dst.H; y++ {
		y0, y1 := y*src.H/dst.H, max((y+1)*src.H/dst.H, y*src.H/dst.H+1)
		for x := 0; x < dst.W; x++ {
			x0, x1 := x*src.W/dst.W, max((x+1)*src.W/dst.W, x*src.W/dst.W+1)
			var r, g, b float32
			for yy := y0; yy < y1; yy++ {
				for xx := x0; xx < x1; xx++ {
					c := src.At(xx, yy)
					r, g, b = r+c.R, g+c.G, b+c.B
				}
			}
			n := float32((y1 - y0) * (x1 - x0))
			dst.Set(x, y, RGB{r / n, g / n, b / n})
		}
	}
}
