package decker

import "math"

// Trim hides the outer fractions of an element's bounds, from each side in:
// {Right: 0.25} shows the left three quarters. The zero Trim shows it all, and
// one whose opposite sides add up to 1 or more shows nothing, glow included.
type Trim struct{ Left, Top, Right, Bottom float64 }

// Composite draws a group of things as one: onto a layer of its own, then
// back onto the canvas faded, moved, scaled and clipped as a whole. The zero
// Composite is invisible (Alpha 0); start from [Identity] and set what you
// want, or take one from the element animations ([FadeIn], [FlyIn], ...).
//
// A Composite is a plain value with no pointers, so building one every frame
// allocates nothing, and effects compose with [Composite.Then]. Positions
// are relative to the bounds given to Draw, and Trim follows the element as
// it moves and scales.
type Composite struct {
	Alpha  float64 // 0 hides the element, 1 draws it as is
	DX, DY float64 // offset in pixels
	Scale  float64 // size about the pivot; 0 means 1

	// PivotX and PivotY place the point Scale grows from, as fractions of
	// the bounds' width and height from their center: 0, 0 scales about the
	// center and -0.5, -0.5 about the top-left corner.
	PivotX, PivotY float64

	Trim Trim // the part of the bounds left out, after moving and scaling
}

// Identity returns the Composite that changes nothing: it draws straight onto
// the canvas, with no layer.
func Identity() Composite { return Composite{Alpha: 1, Scale: 1} }

// scale is Scale with the zero value read as 1.
func (k Composite) scale() float64 {
	if k.Scale > 0 {
		return k.Scale
	}
	return 1
}

// Then returns k followed by o as one Composite: alphas and scales multiply,
// offsets add, trims keep the larger cut per side, and o's pivot wins if it
// sets one. A zero Scale counts as 1; a zero Alpha stays 0.
func (k Composite) Then(o Composite) Composite {
	k.Alpha *= o.Alpha
	k.DX += o.DX
	k.DY += o.DY
	k.Scale = k.scale() * o.scale()
	if o.PivotX != 0 || o.PivotY != 0 {
		k.PivotX, k.PivotY = o.PivotX, o.PivotY
	}
	k.Trim = Trim{
		max(k.Trim.Left, o.Trim.Left), max(k.Trim.Top, o.Trim.Top),
		max(k.Trim.Right, o.Trim.Right), max(k.Trim.Bottom, o.Trim.Bottom),
	}
	return k
}

// Combine runs composites together as Then does, starting from Identity.
func Combine(fxs ...Composite) Composite {
	out := Identity()
	for _, f := range fxs {
		out = out.Then(f)
	}
	return out
}

// Clipped returns k showing only the part of bounds inside clip, in bounds'
// own coordinates (before k moves it). It sets Trim, replacing any there.
func (k Composite) Clipped(bounds, clip Rect) Composite {
	if bounds.W <= 0 || bounds.H <= 0 {
		return k
	}
	k.Trim = Trim{
		Left:   Clamp01((clip.X - bounds.X) / bounds.W),
		Top:    Clamp01((clip.Y - bounds.Y) / bounds.H),
		Right:  Clamp01((bounds.Right() - clip.Right()) / bounds.W),
		Bottom: Clamp01((bounds.Bottom() - clip.Bottom()) / bounds.H),
	}
	return k
}

// Draw calls draw to paint the element, which lives inside bounds (in
// canvas pixels), and puts the result on p as k says. Like a morphing
// element, draw may stray up to a tenth of the canvas height outside bounds
// (a glow, say) and no further; what it draws beyond that is lost.
//
// An identity k calls draw on p itself: no layer, no cost. Fading or
// trimming alone draws on a pooled copy of the region and mixes it back.
// Moving or scaling draws the element twice, onto black and onto white, to
// learn how much of each pixel it covers (the canvas has no alpha), then
// resamples that: bilinear when moving or growing, averaged over the pixels
// each one covers when shrinking. So draw must be pure and may run twice, and
// an element moved off whole pixels is slightly soft.
//
// The layers are pooled and only the region around bounds is cleared and
// composited, so a steady-state frame does not allocate. They are canvas
// sized, because draw paints in canvas coordinates.
func (k Composite) Draw(p *Pixels, bounds Rect, draw func(p *Pixels)) {
	g := k.Alpha
	if !(g > 0) {
		return
	}
	g = min(g, 1)
	s := k.scale()
	moved := k.DX != 0 || k.DY != 0 || s != 1
	if g >= 1 && !moved && k.Trim == (Trim{}) {
		draw(p)
		return
	}
	if bounds.W <= 0 || bounds.H <= 0 {
		return
	}
	margin := float64(p.H) / 10
	sx0, sy0, sx1, sy1 := p.Box(bounds.X-margin, bounds.Y-margin, bounds.Right()+margin, bounds.Bottom()+margin)
	if sx0 > sx1 || sy0 > sy1 {
		return
	}

	// Where the element's bounds land, and the window the trim leaves of them.
	px, py := bounds.X+bounds.W*(0.5+k.PivotX), bounds.Y+bounds.H*(0.5+k.PivotY)
	tx0, ty0 := px+(bounds.X-px)*s+k.DX, py+(bounds.Y-py)*s+k.DY
	tw, th := bounds.W*s, bounds.H*s
	const open = 1e9
	win := [4]float64{-open, -open, open, open}
	// On a trimmed axis the window follows the bounds: a trimmed side sits at
	// its cut, and the other side reaches past the bounds (for a glow) only
	// as far as the share of the axis left showing, so nothing strays out of
	// a side that is hidden, and a fully cut axis shows nothing at all.
	reach := margin * s
	l, r := Clamp01(k.Trim.Left), Clamp01(k.Trim.Right)
	if l+r >= 1 {
		return
	}
	if l+r > 0 {
		win[0], win[2] = tx0-reach*(1-l-r), tx0+tw+reach*(1-l-r)
		if l > 0 {
			win[0] = tx0 + l*tw
		}
		if r > 0 {
			win[2] = tx0 + tw - r*tw
		}
	}
	t, b := Clamp01(k.Trim.Top), Clamp01(k.Trim.Bottom)
	if t+b >= 1 {
		return
	}
	if t+b > 0 {
		win[1], win[3] = ty0-reach*(1-t-b), ty0+th+reach*(1-t-b)
		if t > 0 {
			win[1] = ty0 + t*th
		}
		if b > 0 {
			win[3] = ty0 + th - b*th
		}
	}

	src := [4]int{sx0, sy0, sx1, sy1}
	if moved {
		k.resample(p, src, win, g, s, px, py, draw)
		return
	}
	// Faded or trimmed in place: only what the window leaves of the region.
	x0, y0, x1, y1 := p.Box(max(float64(sx0), win[0])-1, max(float64(sy0), win[1])-1, min(float64(sx1+1), win[2])+1, min(float64(sy1+1), win[3])+1)
	if x0 > x1 || y0 > y1 {
		return
	}
	k.fade(p, src, [4]int{x0, y0, x1, y1}, win, g, draw)
}

// coverSpan is how much of the pixel [v, v+1] lies in [lo, hi].
func coverSpan(v, lo, hi float64) float64 { return Clamp01(min(v+1, hi) - max(v, lo)) }

// fade draws on a copy of p's region src and mixes it back into dst at
// weight g within the window win: the weighted "over" of a layer, exactly,
// without needing its coverage.
func (k Composite) fade(p *Pixels, src, dst [4]int, win [4]float64, g float64, draw func(p *Pixels)) {
	top := layer(p)
	defer layers.put(top)
	for y := src[1]; y <= src[3]; y++ {
		copy(top.Pix[y*p.W+src[0]:y*p.W+src[2]+1], p.Pix[y*p.W+src[0]:y*p.W+src[2]+1])
	}
	top.BG = p.BG
	draw(top)
	for y := max(dst[1], src[1]); y <= min(dst[3], src[3]); y++ {
		wy := g * coverSpan(float64(y), win[1], win[3])
		if wy <= 0 {
			continue
		}
		for x := max(dst[0], src[0]); x <= min(dst[2], src[2]); x++ {
			w := wy * coverSpan(float64(x), win[0], win[2])
			if w <= 0 {
				continue
			}
			i := y*p.W + x
			if w >= 1 {
				p.Pix[i] = top.Pix[i]
			} else {
				p.Pix[i] = Mix(p.Pix[i], top.Pix[i], w)
			}
		}
	}
}

// resample draws the element onto a black layer and a white one, which
// together say each pixel's color and coverage, and composites them back
// moved and scaled. A pixel the element covers by a is a*color on black and
// a*color+(1-a)*255 on white, so the two differ by (1-a)*255 whatever the
// color. Both are linear, so they filter like any image.
func (k Composite) resample(p *Pixels, src [4]int, win [4]float64, g, s, px, py float64, draw func(p *Pixels)) {
	black, white := layer(p), layer(p)
	defer layers.put(black)
	defer layers.put(white)
	n := src[2] - src[0] + 1
	for y := src[1]; y <= src[3]; y++ {
		row := y*p.W + src[0]
		clear(black.Pix[row : row+n])
		w := white.Pix[row : row+n]
		for i := range w {
			w[i] = RGB{255, 255, 255}
		}
	}
	black.BG, white.BG = p.BG, p.BG
	draw(black)
	draw(white)

	// Most of the region around the bounds is empty: shrink it to what the
	// element drew, so the resampling below only visits that.
	const empty, full = 0, 255
	x0, y0, x1, y1 := src[2]+1, src[3]+1, src[0]-1, src[1]-1
	for y := src[1]; y <= src[3]; y++ {
		row := y * p.W
		for x := src[0]; x <= src[2]; x++ {
			if c, d := black.Pix[row+x], white.Pix[row+x]; c != (RGB{empty, empty, empty}) || d != (RGB{full, full, full}) {
				x0, x1, y1 = min(x0, x), max(x1, x), y
				if y0 > y {
					y0 = y
				}
			}
		}
	}
	if x0 > x1 {
		return
	}
	box := [4]int{x0, y0, x1, y1}

	// Where that lands, within the window and a pixel's reach of the filter.
	fwd := func(v, pivot, d float64) float64 { return pivot + (v-pivot)*s + d }
	dx0, dy0, dx1, dy1 := p.Box(
		max(fwd(float64(x0), px, k.DX), win[0])-1, max(fwd(float64(y0), py, k.DY), win[1])-1,
		min(fwd(float64(x1+1), px, k.DX), win[2])+1, min(fwd(float64(y1+1), py, k.DY), win[3])+1)
	if dx0 > dx1 || dy0 > dy1 {
		return
	}

	// Shrinking by more than half needs more than the four bilinear taps to
	// not alias; taps×taps of them across the pixel's footprint is a box
	// filter.
	taps := 1
	if s < 1 {
		taps = min(int(math.Ceil(1/s)), 4)
	}
	sc := scratch.get(func(*tapScratch) bool { return true })
	if sc == nil {
		sc = new(tapScratch)
	}
	defer scratch.put(sc)
	cols := axisTaps(&sc.cols, dx0, dx1, taps, px, k.DX, s, box[0], box[2])
	rows := axisTaps(&sc.rows, dy0, dy1, taps, py, k.DY, s, box[1], box[3])

	wt := 1 / float32(taps*taps)
	for y := dy0; y <= dy1; y++ {
		wy := g * coverSpan(float64(y), win[1], win[3])
		if wy <= 0 {
			continue
		}
		ry := rows[(y-dy0)*taps : (y-dy0+1)*taps]
		for x := dx0; x <= dx1; x++ {
			gw := wy * coverSpan(float64(x), win[0], win[2])
			if gw <= 0 {
				continue
			}
			cx := cols[(x-dx0)*taps : (x-dx0+1)*taps]
			var a, b RGB
			var seen float32
			for _, r := range ry {
				for _, c := range cx {
					seen += wt * (r.wa + r.wb) * (c.wa + c.wb)
					for _, t := range [2]struct {
						row int32
						w   float32
					}{{r.a, r.wa}, {r.b, r.wb}} {
						if t.w == 0 {
							continue
						}
						base := int(t.row) * p.W
						if w := wt * t.w * c.wa; w != 0 {
							accumulate(&a, &b, black.Pix[base+int(c.a)], white.Pix[base+int(c.a)], w)
						}
						if w := wt * t.w * c.wb; w != 0 {
							accumulate(&a, &b, black.Pix[base+int(c.b)], white.Pix[base+int(c.b)], w)
						}
					}
				}
			}
			// Taps off the element see nothing: 0 on black, 255 on white.
			if miss := 1 - seen; miss > 0 {
				b.R, b.G, b.B = b.R+255*miss, b.G+255*miss, b.B+255*miss
			}
			ix := y*p.W + x
			p.Pix[ix] = over(p.Pix[ix], a, b, float32(gw))
		}
	}
}

// accumulate adds w times a layer pixel, given on black (c) and on white (d),
// to the running sums a and b.
func accumulate(a, b *RGB, c, d RGB, w float32) {
	a.R, a.G, a.B = a.R+c.R*w, a.G+c.G*w, a.B+c.B*w
	b.R, b.G, b.B = b.R+d.R*w, b.G+d.G*w, b.B+d.B*w
}

// axisTap is one source sample for a destination pixel along one axis: the
// pixels a and b either side of the sample point, and their linear weights. A
// weight is 0 where the pixel lies off the element's box, and then the index
// is clamped into the box so it can be read anyway.
type axisTap struct {
	a, b   int32
	wa, wb float32
}

// tapScratch holds the per-column and per-row taps of a resample.
type tapScratch struct{ cols, rows []axisTap }

var scratch = sizedPool[tapScratch]{max: 4}

// axisTaps fills *buf with the taps for destination pixels lo..hi along one
// axis, n sub-samples each, and returns it. The map back to the source is
// separable (no rotation), so it is done once per column and per row instead
// of once per pixel. pivot, d and s are the move's pivot, offset and scale on
// this axis; first..last are the source pixels the element covers.
func axisTaps(buf *[]axisTap, lo, hi, n int, pivot, d, s float64, first, last int) []axisTap {
	size := (hi - lo + 1) * n
	if cap(*buf) < size {
		*buf = make([]axisTap, size)
	}
	out := (*buf)[:size]
	inv := 1 / s
	for v := lo; v <= hi; v++ {
		for i := range n {
			dv := float64(v) + (float64(i)+0.5)/float64(n)
			u := pivot + (dv-pivot-d)*inv - 0.5
			fu := math.Floor(u)
			f := u - fu
			// Snap away float noise, so a whole-pixel move reads one pixel.
			if f < 1e-6 {
				f = 0
			} else if f > 1-1e-6 {
				fu, f = fu+1, 0
			}
			t := axisTap{a: int32(fu), b: int32(fu) + 1, wa: float32(1 - f), wb: float32(f)}
			if int(t.a) < first || int(t.a) > last {
				t.wa, t.a = 0, int32(min(max(int(t.a), first), last))
			}
			if int(t.b) < first || int(t.b) > last {
				t.wb, t.b = 0, int32(min(max(int(t.b), first), last))
			}
			out[(v-lo)*n+i] = t
		}
	}
	return out
}

// layerCoverage is how much of a channel a layer pixel covers, from its value
// on black (a) and on white (b).
func layerCoverage(a, b float32) float32 {
	c := 1 - (b-a)*(1.0/255)
	if c < 0 {
		return 0
	}
	if c > 1 {
		return 1
	}
	return c
}

// over puts a layer pixel, given as its color on black (a) and on white (b),
// over d at weight g. Coverage is judged per channel, so additive light (a
// glow) on a layer lands as a screen blend instead of clipping.
func over(d, a, b RGB, g float32) RGB {
	return RGB{
		a.R*g + d.R*(1-layerCoverage(a.R, b.R)*g),
		a.G*g + d.G*(1-layerCoverage(a.G, b.G)*g),
		a.B*g + d.B*(1-layerCoverage(a.B, b.B)*g),
	}
}
