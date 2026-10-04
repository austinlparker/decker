package decker

import (
	"image"
	"sync"
)

// previewKey names one preview: a slide at a step in a pw×ph box, drawn for a
// dw×dh deck.
type previewKey struct{ slide, step, pw, ph, dw, dh int }

// previewMu serializes slide drawing: slides are written for one caller at a
// time, and image previews draw in the background.
var previewMu sync.Mutex

// previewBox is the inside size of each preview frame: the deck's shape,
// leaving a few lines for notes. It is 0, 0 when previews don't fit.
func (p presenter) previewBox() (pw, ph int) {
	inner, rest := max(p.w-2*presMargin, 10), p.h-2-footerLines // 2: header and blank line
	dw, dh := p.deckSize()
	pw = (inner - presGutter - 4) / 2 // each frame adds 2 columns
	// A cell shows one pixel across and two down, like the deck's, so the
	// deck's shape in cells carries over directly.
	ph = pw * dh / dw
	if most := rest - 3 - 6; ph > most { // label + frame, and 6 lines of notes
		ph = most
		pw = ph * dw / dh
	}
	if ph < 5 || pw < 20 {
		return 0, 0
	}
	return pw, ph
}

// settledGrid draws s at its last moment of step, as cells.
func settledGrid(s Slide, step, w, h int, t *Theme) *grid {
	previewMu.Lock()
	defer previewMu.Unlock()
	return renderSlideGrid(s, Ctx{W: w, H: h, T: Settled, Step: step, StepT: Settled, Theme: t})
}

// renderPreview draws s at a size it's designed for (240 cells wide, in the
// deck's shape), then shrinks it into pw×ph cells; drawing at preview size
// would lay the slide out for a tiny screen instead.
func renderPreview(s Slide, step, dw, dh, pw, ph int, t *Theme) string {
	const rw = 240
	rh := max(rw*dh/dw, 20)
	g := settledGrid(s, step, rw, rh, t)
	defer g.release()
	sc := NewScene(pw, ph, t)
	shrinkInto(g.pixels(), sc.Px)
	return sc.Render()
}

// slideImage draws s settled at the deck's size, one image pixel per canvas
// pixel.
func slideImage(s Slide, step, dw, dh int, t *Theme) *image.RGBA {
	g := settledGrid(s, step, dw, dh, t)
	defer g.release()
	px := g.pixels()
	img := image.NewRGBA(image.Rect(0, 0, px.W, px.H))
	for i, c := range px.Pix {
		q := c.q()
		img.Pix[4*i], img.Pix[4*i+1], img.Pix[4*i+2], img.Pix[4*i+3] = q[0], q[1], q[2], 255
	}
	return img
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
