package decker

import "math"

// The transitions that rework pixels instead of moving the frames: fades,
// an iris, a zoom, pixelation and a glitch. Sizes (blocks, slices, bands)
// come from the frame's width, so each looks the same at 240 cells and at
// video resolution. None has a direction.

// blend mixes from into to at weight e: e <= 0 is all from, e >= 1 leaves to.
func blend(from, to *Pixels, e float64) {
	switch {
	case e <= 0:
		copy(to.Pix, from.Pix)
	case e < 1:
		for i, c := range to.Pix {
			to.Pix[i] = Mix(from.Pix[i], c, e)
		}
	}
}

// crossfade fades the pixels from one frame to the other; characters can't
// fade, so they switch half-way.
func crossfade(from, to *Scene, p float64, _ Direction, _ *Theme) {
	e := EaseInOutCubic(p)
	blend(from.Px, to.Px, e)
	moveChars(from, to, func(x, y int, old bool) (int, int, bool) { return x, y, old == (e < 0.5) })
}

// fadeThrough fades the old frame to the theme's background and the new one
// in from it. Characters are only there while their frame is more than half
// visible, which keeps them from floating over the dark.
func fadeThrough(from, to *Scene, p float64, _ Direction, t *Theme) {
	var showOld, showNew bool
	if p < 0.5 {
		a := EaseInOutCubic(2 * p)
		showOld = a < 0.5
		for i, c := range from.Px.Pix {
			to.Px.Pix[i] = Mix(c, t.Background, a)
		}
	} else if b := EaseInOutCubic(2*p - 1); b < 1 {
		showNew = b >= 0.5
		for i, c := range to.Px.Pix {
			to.Px.Pix[i] = Mix(t.Background, c, b)
		}
	} else {
		showNew = true
	}
	moveChars(from, to, func(x, y int, old bool) (int, int, bool) { return x, y, old && showOld || !old && showNew })
}

// iris reveals the new frame inside a circle that grows from the center
// until it passes the corners. The pixels are square, so the circle is round;
// its edge is antialiased a pixel wide, and a character shows the frame its
// cell's center is in.
func iris(from, to *Scene, p float64, _ Direction, _ *Theme) {
	w, h := to.W, to.Px.H
	cx, cy := float64(w)/2, float64(h)/2
	// Start and end half a pixel off the circle so p = 0 and p = 1 are exactly
	// the old and the new frame.
	r := -0.5 + (math.Hypot(cx, cy)+1)*EaseInOutCubic(p)
	in, out := max(r-0.5, 0), r+0.5
	pix, src := to.Px.Pix, from.Px.Pix
	for y := range h {
		dy := float64(y) + 0.5 - cy
		for x := range w {
			dx := float64(x) + 0.5 - cx
			d2 := dx*dx + dy*dy
			i := y*w + x
			switch {
			case r > 0.5 && d2 <= in*in:
			case d2 >= out*out:
				pix[i] = src[i]
			default:
				pix[i] = Mix(src[i], pix[i], Coverage(math.Sqrt(d2)-r))
			}
		}
	}
	moveChars(from, to, func(x, y int, old bool) (int, int, bool) {
		dx, dy := float64(x)+0.5-cx, float64(2*y+1)-cy
		return x, y, old != (r > 0 && dx*dx+dy*dy < r*r)
	})
}

// Zoom scales: the old frame grows by zoomOld at most and the new one
// starts zoomNew smaller than full size.
const (
	zoomOld = 0.25
	zoomNew = 0.15
)

// zoom scales the old frame up about the center as it fades out while the
// new one grows from slightly small to full size, fading in. Characters stay
// on their cells and switch half-way.
func zoom(from, to *Scene, p float64, _ Direction, _ *Theme) {
	e := EaseInOutCubic(p)
	if e <= 0 {
		copy(to.Px.Pix, from.Px.Pix)
	} else if e < 1 {
		zoomPixels(from.Px, to.Px, e)
	}
	moveChars(from, to, func(x, y int, old bool) (int, int, bool) { return x, y, old == (e < 0.5) })
}

// zoomTaps are where each column of a zoomed frame samples the source: the
// left of two taps and the weight of the right one, for the old frame (o) and
// the new (n), and how much of the new frame covers the column. The mapping
// is the same for every row, so it is worked out once.
type zoomTaps struct {
	oi, ni []int32
	of, nf []float32
	cover  []float32
}

var zoomTapPool = sizedPool[zoomTaps]{max: 2}

// tap returns the left tap and the weight of the right one for position u,
// in pixel-index coordinates, on an axis of n pixels, clamped to the edge.
func tap(u float64, n int) (int32, float32) {
	u = min(max(u, 0), float64(n-1))
	i := int(u)
	return int32(i), float32(u - float64(i))
}

// zoomPixels resamples both frames bilinearly and mixes them by e, writing
// to's pixels, so it first copies them. The new frame, smaller than the
// canvas, fades into the old one at its edge.
func zoomPixels(from, to *Pixels, e float64) {
	w, h := to.W, to.H
	src := getScratch(len(to.Pix))
	defer putScratch(src)
	copy(src.Pix, to.Pix)
	tb := zoomTapPool.get(func(t *zoomTaps) bool { return cap(t.oi) >= w })
	if tb == nil {
		zoomTapPool.drain()
		tb = &zoomTaps{make([]int32, w), make([]int32, w), make([]float32, w), make([]float32, w), make([]float32, w)}
	}
	defer zoomTapPool.put(tb)
	so, sn := 1+zoomOld*e, 1-zoomNew*(1-e)
	cx, cy := float64(w)/2, float64(h)/2
	// edge is how far inside the new frame's edge a position on an axis of n
	// pixels is, in canvas pixels, as coverage.
	edge := func(u float64, n int) float32 {
		return float32(Clamp01(min(u+0.5, float64(n)-0.5-u)*sn + 0.5))
	}
	for x := range w {
		dx := float64(x) + 0.5 - cx
		un := cx + dx/sn - 0.5
		tb.oi[x], tb.of[x] = tap(cx+dx/so-0.5, w)
		tb.ni[x], tb.nf[x] = tap(un, w)
		tb.cover[x] = edge(un, w)
	}
	qe := float32(e)
	for y := range h {
		dy := float64(y) + 0.5 - cy
		vo, vn := cy+dy/so-0.5, cy+dy/sn-0.5
		yo, fyo := tap(vo, h)
		yn, fyn := tap(vn, h)
		coverY := edge(vn, h)
		o0, o1 := from.Pix[int(yo)*w:], from.Pix[min(int(yo)+1, h-1)*w:]
		n0, n1 := src.Pix[int(yn)*w:], src.Pix[min(int(yn)+1, h-1)*w:]
		out := to.Pix[y*w : (y+1)*w]
		for x := range out {
			i, f := int(tb.oi[x]), tb.of[x]
			c := lerpRows(o0, o1, i, min(i+1, w-1), f, fyo)
			if q := qe * tb.cover[x] * coverY; q > 0 {
				i, f = int(tb.ni[x]), tb.nf[x]
				n := lerpRows(n0, n1, i, min(i+1, w-1), f, fyn)
				c = RGB{c.R + (n.R-c.R)*q, c.G + (n.G-c.G)*q, c.B + (n.B-c.B)*q}
			}
			out[x] = c
		}
	}
}

// lerpRows blends the 2×2 pixels at columns i, j of rows r0, r1: fx of the
// way from column i to j, then fy from r0 to r1.
func lerpRows(r0, r1 []RGB, i, j int, fx, fy float32) RGB {
	a, b, c, d := r0[i], r0[j], r1[i], r1[j]
	tr, tg, tb := a.R+(b.R-a.R)*fx, a.G+(b.G-a.G)*fx, a.B+(b.B-a.B)*fx
	br, bg, bb := c.R+(d.R-c.R)*fx, c.G+(d.G-c.G)*fx, c.B+(d.B-c.B)*fx
	return RGB{tr + (br-tr)*fy, tg + (bg-tg)*fy, tb + (bb-tb)*fy}
}

// pixelate breaks the old frame into blocks that grow to a twentieth of the
// width, cross-fades to the new frame's blocks around the peak, and has them
// shrink back to pixels. Characters are too fine for blocks, so they show
// only at the two ends.
func pixelate(from, to *Scene, p float64, _ Direction, _ *Theme) {
	w, h := to.W, to.Px.H
	bs := 1 + int(EaseInOutCubic(1-math.Abs(1-2*p))*float64(max(w/20, 2)-1)+0.5)
	m := Clamp01((p - 0.4) / 0.2)
	m *= m * (3 - 2*m) // how much of the new frame's blocks are in the mix
	switch {
	case bs > 1:
		for y0 := 0; y0 < h; y0 += bs {
			y1 := min(y0+bs, h)
			for x0 := 0; x0 < w; x0 += bs {
				x1 := min(x0+bs, w)
				var c RGB
				if m < 1 {
					c = blockAverage(from.Px.Pix, w, x0, y0, x1, y1)
				}
				if m > 0 {
					n := blockAverage(to.Px.Pix, w, x0, y0, x1, y1)
					c = Mix(c, n, m)
					if m >= 1 {
						c = n
					}
				}
				for y := y0; y < y1; y++ {
					row := to.Px.Pix[y*w+x0 : y*w+x1]
					for i := range row {
						row[i] = c
					}
				}
			}
		}
	case m <= 0:
		copy(to.Px.Pix, from.Px.Pix)
	}
	moveChars(from, to, func(x, y int, old bool) (int, int, bool) { return x, y, old && p < 0.2 || !old && p > 0.8 })
}

// blockAverage is the mean color of the pixels in [x0,x1)×[y0,y1).
func blockAverage(pix []RGB, w, x0, y0, x1, y1 int) RGB {
	var r, g, b float32
	for y := y0; y < y1; y++ {
		for _, c := range pix[y*w+x0 : y*w+x1] {
			r, g, b = r+c.R, g+c.G, b+c.B
		}
	}
	n := float32((x1 - x0) * (y1 - y0))
	return RGB{r / n, g / n, b / n}
}

// glitchSteps is how many times a glitch's noise changes over a transition.
const glitchSteps = 14

// glitchSlice is what the slice of the frame numbered sl does at progress p:
// whether it shows the new frame (each slice has its own moment, jittered
// from step to step), and, if it is torn, how many cells it is shifted
// sideways. Both depend on p and Hash01 only, and the noise dies away at 0
// and 1 so the ends are clean.
func glitchSlice(sl int, p float64, w int) (useNew bool, off int, torn bool) {
	p = Clamp01(p)
	step := int(p * glitchSteps)
	env := 4 * p * (1 - p)
	useNew = p+(Hash01(sl, step, 42)-0.5)*0.4*env > Hash01(sl, 0, 41)
	if Hash01(sl, step, 43) < 0.75*math.Sqrt(env) {
		off = int((Hash01(sl, step, 44)*2 - 1) * 0.15 * float64(w) * (0.3 + 0.7*env))
		torn = true
	}
	return useNew, off, torn
}

// glitch cuts the frame into horizontal slices, a few cells to a few dozen
// tall by the frame's width, that flip to the new frame at their own moments
// and, while torn, shift sideways with their red and blue channels split
// apart. A character moves with its slice.
func glitch(from, to *Scene, p float64, _ Direction, _ *Theme) {
	w, h := to.W, to.Px.H
	sh := max(w/80, 1)
	split := max(w/100, 2)
	var tmp *Pixels
	for sl := 0; sl*sh < to.H; sl++ {
		useNew, off, torn := glitchSlice(sl, p, w)
		for y := 2 * sl * sh; y < min(2*(sl+1)*sh, h); y++ {
			row := to.Px.Pix[y*w : (y+1)*w]
			src := from.Px.Pix[y*w : (y+1)*w]
			switch {
			case !torn:
				if !useNew {
					copy(row, src)
				}
				continue
			case useNew:
				if tmp == nil {
					tmp = getScratch(w)
				}
				copy(tmp.Pix, row)
				src = tmp.Pix
			}
			splitRow(row, src, off, split)
		}
	}
	if tmp != nil {
		putScratch(tmp)
	}
	moveChars(from, to, func(x, y int, old bool) (int, int, bool) {
		useNew, off, torn := glitchSlice(y/sh, p, w)
		if old == useNew {
			return 0, 0, false
		}
		if torn {
			x = wrapIndex(x+off, w)
		}
		return x, y, true
	})
}

// splitRow writes src shifted right by off cells into dst, wrapping, with
// the red channel taken from split cells further right and the blue from as
// many to the left.
func splitRow(dst, src []RGB, off, split int) {
	w := len(dst)
	r, g, b := wrapIndex(split-off, w), wrapIndex(-off, w), wrapIndex(-split-off, w)
	for x := range dst {
		dst[x] = RGB{src[r].R, src[g].G, src[b].B}
		if r++; r == w {
			r = 0
		}
		if g++; g == w {
			g = 0
		}
		if b++; b == w {
			b = 0
		}
	}
}

// wrapIndex returns i modulo n, in [0, n).
func wrapIndex(i, n int) int { return (i%n + n) % n }
