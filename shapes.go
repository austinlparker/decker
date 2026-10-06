package decker

import "math"

// Scratch buffers for the shapes below. Strokes need a coverage mask and
// curves need point lists, and both are rebuilt every frame, so they are
// borrowed from these pools rather than allocated.
var (
	maskPool = sizedPool[[]float32]{max: 4}
	ptsPool  = sizedPool[[]float64]{max: 8}
)

// getMask returns a zeroed mask of n samples; give it back with putMask. A
// pooled buffer that is too small is replaced by a bigger one that then takes
// its place in the pool, so a full pool of small buffers can't force an
// allocation every frame.
func getMask(n int) *[]float32 {
	m := maskPool.get(func(*[]float32) bool { return true })
	if m == nil {
		m = new([]float32)
	}
	if cap(*m) < n {
		*m = make([]float32, n)
	}
	*m = (*m)[:n]
	clear(*m)
	return m
}

func putMask(m *[]float32) { maskPool.put(m) }

// getPts returns an empty point list for building a path; give it back with
// putPts.
func getPts() *[]float64 {
	if v := ptsPool.get(func(*[]float64) bool { return true }); v != nil {
		*v = (*v)[:0]
		return v
	}
	v := make([]float64, 0, 256)
	return &v
}

func putPts(v *[]float64) { ptsPool.put(v) }

// Polygon fills the closed shape through pts, x0, y0, x1, y1 and so on, with
// antialiased edges. The last point joins back to the first. Concave shapes
// work, and the fill rule is even-odd: a region is inside when a ray from it
// crosses an odd number of edges, so where the outline crosses itself (a
// pentagram) the overlap is a hole. Fewer than three points draw nothing.
func (p *Pixels) Polygon(pts []float64, c RGB, a float64) {
	n := len(pts) / 2
	if n < 3 || a <= 0 {
		return
	}
	minX, minY, maxX, maxY := pts[0], pts[1], pts[0], pts[1]
	for i := 1; i < n; i++ {
		minX, maxX = min(minX, pts[2*i]), max(maxX, pts[2*i])
		minY, maxY = min(minY, pts[2*i+1]), max(maxY, pts[2*i+1])
	}
	x0, y0, x1, y1 := p.Box(minX-1, minY-1, maxX+1, maxY+1)
	// Only edges within half a pixel of a row can change its pixels, whether
	// by distance or by crossing the row, so each row works from that short list.
	var buf [32]int
	for py := y0; py <= y1; py++ {
		fy := float64(py) + 0.5
		act := buf[:0]
		for i := 0; i < n; i++ {
			j := (i + 1) % n
			lo, hi := min(pts[2*i+1], pts[2*j+1]), max(pts[2*i+1], pts[2*j+1])
			if fy >= lo-0.5 && fy <= hi+0.5 {
				act = append(act, i)
			}
		}
		if len(act) == 0 {
			continue
		}
		for px := x0; px <= x1; px++ {
			fx := float64(px) + 0.5
			inside := false
			d2 := math.Inf(1)
			for _, i := range act {
				j := (i + 1) % n
				ax, ay, bx, by := pts[2*i], pts[2*i+1], pts[2*j], pts[2*j+1]
				if (ay > fy) != (by > fy) && fx < ax+(fy-ay)*(bx-ax)/(by-ay) {
					inside = !inside
				}
				d2 = min(d2, segDist2(fx, fy, ax, ay, bx, by))
			}
			if d2 >= 0.25 {
				if inside {
					p.Blend(px, py, c, a)
				}
				continue
			}
			d := math.Sqrt(d2)
			if inside {
				d = -d
			}
			p.Blend(px, py, c, a*Coverage(d))
		}
	}
}

// segDist2 is the squared distance from (px, py) to the segment a-b.
func segDist2(px, py, ax, ay, bx, by float64) float64 {
	dx, dy := bx-ax, by-ay
	t := 0.0
	if l2 := dx*dx + dy*dy; l2 > 0 {
		t = Clamp01(((px-ax)*dx + (py-ay)*dy) / l2)
	}
	ex, ey := px-(ax+t*dx), py-(ay+t*dy)
	return ex*ex + ey*ey
}

// Ellipse fills an antialiased ellipse centered on (cx, cy) with radii rx and
// ry, or only an outline of thickness stroke, drawn inward from the edge, if
// stroke > 0. With rx == ry it is Disc, or a ring.
func (p *Pixels) Ellipse(cx, cy, rx, ry, stroke float64, c RGB, a float64) {
	if rx <= 0 || ry <= 0 {
		return
	}
	x0, y0, x1, y1 := p.Box(cx-rx-1, cy-ry-1, cx+rx+1, cy+ry+1)
	for py := y0; py <= y1; py++ {
		y := float64(py) + 0.5 - cy
		for px := x0; px <= x1; px++ {
			x := float64(px) + 0.5 - cx
			// First-order distance to the implicit curve f=|(x/rx, y/ry)|-1:
			// f/|grad f|. It is exact for a circle and within a fraction of a
			// pixel of the edge for any ellipse, which is where it counts.
			k0 := math.Sqrt(x*x/(rx*rx) + y*y/(ry*ry))
			k1 := math.Sqrt(x*x/(rx*rx*rx*rx) + y*y/(ry*ry*ry*ry))
			var d float64
			if k1 < 1e-12 {
				d = -min(rx, ry)
			} else {
				d = k0 * (k0 - 1) / k1
			}
			if stroke > 0 {
				d = math.Abs(d+stroke/2) - stroke/2
			} else if d <= -0.5 {
				p.Blend(px, py, c, a)
				continue
			}
			p.Blend(px, py, c, a*Coverage(d))
		}
	}
}

// Polyline strokes the path through pts, x0, y0, x1, y1 and so on, with round
// joins and caps. The whole path is covered once, so at a < 1 the joints show
// no dark or bright dots where segments overlap. A single point draws a dot.
func (p *Pixels) Polyline(pts []float64, width float64, c RGB, a float64) {
	p.stroke(pts, width, c, a)
}

// stroke builds one coverage mask for the whole path, taking the max over its
// segments, then blends it once.
func (p *Pixels) stroke(pts []float64, width float64, c RGB, a float64) {
	n := len(pts) / 2
	if n == 0 || a <= 0 || width <= 0 {
		return
	}
	half := width / 2
	minX, minY, maxX, maxY := pts[0], pts[1], pts[0], pts[1]
	for i := 1; i < n; i++ {
		minX, maxX = min(minX, pts[2*i]), max(maxX, pts[2*i])
		minY, maxY = min(minY, pts[2*i+1]), max(maxY, pts[2*i+1])
	}
	x0, y0, x1, y1 := p.Box(minX-half-1, minY-half-1, maxX+half+1, maxY+half+1)
	mw, mh := x1-x0+1, y1-y0+1
	if mw <= 0 || mh <= 0 {
		return
	}
	mp := getMask(mw * mh)
	defer putMask(mp)
	mask := *mp
	reach := (half + 0.5) * (half + 0.5)
	for i := 0; i < max(n-1, 1); i++ {
		ax, ay := pts[2*i], pts[2*i+1]
		bx, by := ax, ay
		if n > 1 {
			bx, by = pts[2*i+2], pts[2*i+3]
		}
		sx0 := max(int(math.Floor(min(ax, bx)-half-1)), x0)
		sy0 := max(int(math.Floor(min(ay, by)-half-1)), y0)
		sx1 := min(int(math.Ceil(max(ax, bx)+half+1)), x1)
		sy1 := min(int(math.Ceil(max(ay, by)+half+1)), y1)
		for py := sy0; py <= sy1; py++ {
			row := mask[(py-y0)*mw:][:mw]
			for px := sx0; px <= sx1; px++ {
				d2 := segDist2(float64(px)+0.5, float64(py)+0.5, ax, ay, bx, by)
				if d2 >= reach {
					continue
				}
				if cv := float32(Coverage(math.Sqrt(d2) - half)); cv > row[px-x0] {
					row[px-x0] = cv
				}
			}
		}
	}
	for py := y0; py <= y1; py++ {
		row := mask[(py-y0)*mw:][:mw]
		for px := x0; px <= x1; px++ {
			if cv := row[px-x0]; cv > 0 {
				p.Blend(px, py, c, a*float64(cv))
			}
		}
	}
}

// Bezier strokes the cubic curve from (x0, y0) to (x1, y1) with control points
// (cx0, cy0) and (cx1, cy1), with round caps. prog (0..1) draws it on from the
// start to that fraction of its length: arc length, to within the accuracy of
// the flattened curve, so the pen moves at an even speed.
func (p *Pixels) Bezier(x0, y0, cx0, cy0, cx1, cy1, x1, y1, width float64, c RGB, a float64, prog float64) {
	if prog <= 0 {
		return
	}
	full, part := getPts(), getPts()
	defer putPts(full)
	defer putPts(part)
	*full = flattenBezier(*full, x0, y0, cx0, cy0, cx1, cy1, x1, y1)
	if prog < 1 {
		*part = pathSpan(*part, *full, 0, prog*pathLen(*full))
		p.stroke(*part, width, c, a)
		return
	}
	p.stroke(*full, width, c, a)
}

// flattenBezier appends a polyline approximating the curve to dst. The segment
// count follows the control polygon's length, about one per 3 pixels.
func flattenBezier(dst []float64, x0, y0, cx0, cy0, cx1, cy1, x1, y1 float64) []float64 {
	l := math.Hypot(cx0-x0, cy0-y0) + math.Hypot(cx1-cx0, cy1-cy0) + math.Hypot(x1-cx1, y1-cy1)
	n := min(max(int(l/3), 16), 256)
	for i := 0; i <= n; i++ {
		t := float64(i) / float64(n)
		u := 1 - t
		b0, b1, b2, b3 := u*u*u, 3*u*u*t, 3*u*t*t, t*t*t
		dst = append(dst, b0*x0+b1*cx0+b2*cx1+b3*x1, b0*y0+b1*cy0+b2*cy1+b3*y1)
	}
	return dst
}

// pathLen is the length of the polyline pts.
func pathLen(pts []float64) float64 {
	l := 0.0
	for i := 2; i+1 < len(pts); i += 2 {
		l += math.Hypot(pts[i]-pts[i-2], pts[i+1]-pts[i-1])
	}
	return l
}

// pathSpan appends to dst the part of the polyline pts between arc lengths
// from and to. A span of zero length yields its single point.
func pathSpan(dst, pts []float64, from, to float64) []float64 {
	if len(pts) < 2 {
		return dst
	}
	from = max(from, 0)
	if to < from {
		return dst
	}
	started, acc := false, 0.0
	for i := 2; i+1 < len(pts); i += 2 {
		ax, ay, bx, by := pts[i-2], pts[i-1], pts[i], pts[i+1]
		l := math.Hypot(bx-ax, by-ay)
		if l == 0 || acc+l < from {
			acc += l
			continue
		}
		if !started {
			f := (from - acc) / l
			dst = append(dst, ax+(bx-ax)*f, ay+(by-ay)*f)
			started = true
		}
		if acc+l >= to {
			f := (to - acc) / l
			return append(dst, ax+(bx-ax)*f, ay+(by-ay)*f)
		}
		dst = append(dst, bx, by)
		acc += l
	}
	if !started {
		dst = append(dst, pts[len(pts)-2], pts[len(pts)-1])
	}
	return dst
}

// pathAt returns the point at arc length s along the polyline pts, clamped to
// its ends.
func pathAt(pts []float64, s float64) (x, y float64) {
	var buf [4]float64
	span := pathSpan(buf[:0], pts, s, s)
	if len(span) < 2 {
		return 0, 0
	}
	return span[len(span)-2], span[len(span)-1]
}

// DashedLine strokes a line of dashes dash long with gap between them, with
// round caps, starting with a dash at (xa, ya). It is the dashing
// PlaceholderBox uses, so the two phase identically.
func (p *Pixels) DashedLine(xa, ya, xb, yb, width, dash, gap float64, c RGB, a float64) {
	l := math.Hypot(xb-xa, yb-ya)
	step := dash + max(gap, 0)
	if dash <= 0 || step < 1 || l/step > 1e5 {
		p.Line(xa, ya, xb, yb, width, c, a)
		return
	}
	for d := 0.0; d < l; d += step {
		e := min(d+dash, l)
		p.Line(xa+(xb-xa)*d/l, ya+(yb-ya)*d/l, xa+(xb-xa)*e/l, ya+(yb-ya)*e/l, width, c, a)
	}
}

// dashedPath strokes the polyline pts as dashes that follow its corners; each
// dash is one stroke, so a corner inside a dash doesn't double-blend.
func (p *Pixels) dashedPath(pts []float64, width, dash, gap float64, c RGB, a float64) {
	total := pathLen(pts)
	step := dash + max(gap, 0)
	if dash <= 0 || step < 1 || total/step > 1e5 {
		p.stroke(pts, width, c, a)
		return
	}
	sub := getPts()
	defer putPts(sub)
	for d := 0.0; d < total; d += step {
		*sub = pathSpan((*sub)[:0], pts, d, min(d+dash, total))
		p.stroke(*sub, width, c, a)
	}
}

// roundRectDist is the signed distance (negative inside) from (x, y) to the
// rounded rectangle centered on (cx, cy) whose corner circles' centers lie
// hx and hy from it.
func roundRectDist(x, y, cx, cy, hx, hy, radius float64) float64 {
	qx, qy := math.Abs(x-cx)-hx, math.Abs(y-cy)-hy
	return math.Hypot(max(qx, 0), max(qy, 0)) + min(max(qx, qy), 0) - radius
}

// gradientFill paints the rounded rectangle with the color color(x, y) gives
// each pixel center.
func (p *Pixels) gradientFill(x, y, w, h, radius float64, a float64, color func(fx, fy float64) RGB) {
	radius = max(min(radius, w/2, h/2), 0)
	x0, y0, x1, y1 := p.Box(x-1, y-1, x+w+1, y+h+1)
	cx, cy := x+w/2, y+h/2
	hx, hy := w/2-radius, h/2-radius
	for py := y0; py <= y1; py++ {
		for px := x0; px <= x1; px++ {
			fx, fy := float64(px)+0.5, float64(py)+0.5
			cv := Coverage(roundRectDist(fx, fy, cx, cy, hx, hy, radius))
			if cv > 0 {
				p.Blend(px, py, color(fx, fy), a*cv)
			}
		}
	}
}

// LinearGradient fills the rounded rectangle at (x, y), w by h (radius 0 for
// square corners), with a color that runs from `from` at (gx0, gy0) to `to` at
// (gx1, gy1) and is constant beyond them.
func (p *Pixels) LinearGradient(x, y, w, h, radius, gx0, gy0, gx1, gy1 float64, from, to RGB, a float64) {
	dx, dy := gx1-gx0, gy1-gy0
	l2 := dx*dx + dy*dy
	p.gradientFill(x, y, w, h, radius, a, func(fx, fy float64) RGB {
		if l2 == 0 {
			return from
		}
		return Mix(from, to, ((fx-gx0)*dx+(fy-gy0)*dy)/l2)
	})
}

// RadialGradient fills the rounded rectangle at (x, y), w by h (radius 0 for
// square corners), with a color that runs from inner at (gx, gy) to outer at
// distance gr from it and is constant beyond.
func (p *Pixels) RadialGradient(x, y, w, h, radius, gx, gy, gr float64, inner, outer RGB, a float64) {
	p.gradientFill(x, y, w, h, radius, a, func(fx, fy float64) RGB {
		if gr <= 0 {
			return outer
		}
		return Mix(inner, outer, math.Hypot(fx-gx, fy-gy)/gr)
	})
}
