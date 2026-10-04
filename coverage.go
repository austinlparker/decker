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
