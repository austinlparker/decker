package decker

import "math"

// coverage is a float alpha mask with its top-left at (x0, y0) on the pixel
// canvas.
type coverage struct {
	a      []float32
	w, h   int
	x0, y0 int
}

func newCoverage(x0, y0, w, h int) coverage {
	return coverage{make([]float32, w*h), w, h, x0, y0}
}

// inkBox is the coverage buffer's place and size for a block of text at size
// whose box is w×h at (x, y): padded by size on every side for glow and
// effect motion.
func inkBox(x, y, w, h float64, size int) (x0, y0, cw, ch int) {
	pad := float64(size)
	return int(math.Floor(x - pad)), int(math.Floor(y - pad)), int(math.Ceil(w+2*pad)) + 1, int(math.Ceil(h+2*pad)) + 1
}

// lineBoxes is the box of each line of a block whose top-left anchor is
// (x, y), as aligned, for a review.
func lineBoxes(a Align, widths []float64, x, y, lineH float64) []Rect {
	boxes := make([]Rect, len(widths))
	for i, w := range widths {
		boxes[i] = Rect{x + a.shift(w), y + float64(i)*lineH, w, lineH}
	}
	return boxes
}

// penAt splits a pen position into the whole pixel a glyph is placed from
// and which of its subpixel renderings covers the rest.
func penAt(x float64) (ix float64, q int) {
	ix = math.Floor(x)
	return ix, int((x - ix) * subpixel)
}

// stampGlyph stamps r from f at size with its pen at (gx, gy) in mask
// coordinates, as fx says. adv is r's advance: a stand-in fx.Rune is
// centered in it. It returns gx moved by that centering.
func (c coverage) stampGlyph(f *Font, size int, r rune, adv, gx, gy float64, fx GlyphFX) float64 {
	show := r
	if fx.Rune != 0 && r != ' ' {
		show = fx.Rune
		gx += (adv - f.glyph(show, size).adv) / 2
	}
	ix, q := penAt(gx)
	c.stamp(f.glyphAt(show, size, q), ix, gy, fx.Alpha)
	return gx
}

// stamp adds a glyph's coverage with its pen at (penX, baseline penY) in
// mask coordinates, taking the max with what's there.
func (c coverage) stamp(g *glyph, penX, penY, alpha float64) {
	if g.a == nil || alpha <= 0 {
		return
	}
	ox := int(math.Round(penX)) + g.ox
	oy := int(math.Round(penY)) + g.oy
	al := float32(Clamp01(alpha)) / 255
	for y := 0; y < g.h; y++ {
		ty := oy + y
		if ty < 0 || ty >= c.h {
			continue
		}
		for x := 0; x < g.w; x++ {
			tx := ox + x
			if tx < 0 || tx >= c.w {
				continue
			}
			v := float32(g.a[y*g.w+x]) * al
			if i := ty*c.w + tx; v > c.a[i] {
				c.a[i] = v
			}
		}
	}
}

// paintFlat blends the mask onto p in col.
func (c coverage) paintFlat(p *Pixels, col RGB) {
	for y := 0; y < c.h; y++ {
		for x := 0; x < c.w; x++ {
			if a := c.a[y*c.w+x]; a > 0.002 {
				p.Blend(c.x0+x, c.y0+y, col, float64(a))
			}
		}
	}
}

// addGlow blurs the mask by radius and adds it to the pixel layer as light.
func (c coverage) addGlow(p *Pixels, radius int, col RGB, strength float64) {
	blur := boxBlur(c.a, c.w, c.h, radius)
	for y := 0; y < c.h; y++ {
		for x := 0; x < c.w; x++ {
			if v := blur[y*c.w+x]; v > 0.003 {
				p.Add(c.x0+x, c.y0+y, col, float64(v)*strength)
			}
		}
	}
}

// boxBlur approximates a Gaussian with two box-blur passes per direction.
func boxBlur(src []float32, w, h, r int) []float32 {
	a := append([]float32(nil), src...)
	b := make([]float32, len(src))
	for range 2 {
		blur1D(a, b, w, h, 1, w, r)
		blur1D(b, a, h, w, w, 1, r)
	}
	return a
}

// blur1D box-blurs m lines of n samples from src into dst; samples are si apart
// within a line, lines sj apart, and out-of-range samples count as zero.
func blur1D(src, dst []float32, n, m, si, sj, r int) {
	norm := 1 / float32(2*r+1)
	for j := range m {
		base := j * sj
		var sum float32
		for i := 0; i <= min(r, n-1); i++ {
			sum += src[base+i*si]
		}
		for i := range n {
			dst[base+i*si] = sum * norm
			if out := i - r; out >= 0 {
				sum -= src[base+out*si]
			}
			if in := i + r + 1; in < n {
				sum += src[base+in*si]
			}
		}
	}
}

// fillRect adds an axis-aligned rectangle in mask coordinates, edges
// antialiased by area, taking the max with what's there.
func (c coverage) fillRect(x0, y0, x1, y1, alpha float64) {
	al := float32(Clamp01(alpha))
	if al <= 0 || x1 <= x0 || y1 <= y0 {
		return
	}
	ix0, iy0 := max(int(math.Floor(x0)), 0), max(int(math.Floor(y0)), 0)
	ix1, iy1 := min(int(math.Ceil(x1)), c.w), min(int(math.Ceil(y1)), c.h)
	for y := iy0; y < iy1; y++ {
		cy := float32(min(float64(y+1), y1) - max(float64(y), y0))
		for x := ix0; x < ix1; x++ {
			cx := float32(min(float64(x+1), x1) - max(float64(x), x0))
			if v := cx * cy * al; v > c.a[y*c.w+x] {
				c.a[y*c.w+x] = v
			}
		}
	}
}
