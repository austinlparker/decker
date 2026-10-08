package decker

import "image"

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
	if p.images != nil {
		pw = min(pw, kittyMaxCells)
	}
	// A cell shows one pixel across and two down, like the deck's, so the
	// deck's shape in cells carries over directly.
	ph := pw * dh / dw
	most := p.h - presHeader - footerLines - 3 - 6 // label + frame, and 6 lines of notes
	if p.images != nil {
		most = min(most, kittyMaxCells)
	}
	if ph > most {
		ph = most
		pw = ph * dw / dh
	}
	return previewKey{i, step, pw, ph, dw, dh}, ph >= 5 && pw >= 20
}

// renderPreview draws slide k.slide of slides at a size it's designed for (240
// cells wide, in the deck's shape), then shrinks it into pw×ph cells; drawing
// at preview size would lay the slide out for a tiny screen instead.
func renderPreview(slides []Slide, k previewKey, t *Theme) string {
	const rw = 240
	rh := max(rw*k.dh/k.dw, 20)
	g := renderSlideGrid(slides[k.slide], Ctx{W: rw, H: rh, T: Settled, Step: k.step, StepT: Settled, Theme: t}.at(slides, k.slide))
	defer g.release()
	sc := NewScene(k.pw, k.ph, t)
	px := g.pixels()
	boxScale(sc.Px.Pix, sc.Px.W, sc.Px.H, px.Pix, px.W, px.H)
	return sc.Render()
}

// slideImage preserves the live deck's layout and slide metadata. Pixel-only
// slides need just one image pixel per canvas pixel; native character layers
// use the snapshot rasterizer so their glyphs remain readable too.
func slideImage(slides []Slide, k previewKey, t *Theme) *image.RGBA {
	g := renderSlideGrid(slides[k.slide], Ctx{W: k.dw, H: k.dh, T: Settled, Step: k.step, StepT: Settled, Theme: t}.at(slides, k.slide))
	defer g.release()
	for _, c := range g.Cells {
		if c.ch != " " && c.ch != "" && c.ch != halfBlock {
			return frameImage(g)
		}
	}
	px := g.pixels()
	img := image.NewRGBA(image.Rect(0, 0, px.W, px.H))
	for i, c := range px.Pix {
		q := c.q()
		img.Pix[4*i], img.Pix[4*i+1], img.Pix[4*i+2], img.Pix[4*i+3] = q[0], q[1], q[2], 255
	}
	return img
}
