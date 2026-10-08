package decker

import "math"

// Rect is a box on the pixel canvas: X, Y is its top-left corner and W, H its
// size, all in pixels. Layout is cutting rects out of rects: start from
// Ctx.Frame or Ctx.Rect, then Inset, Cut, Rows, Cols, Grid or Anchor, and hand the
// pieces to whatever draws there. Every method returns new rects; widths and
// heights never go below zero. Intersect, Union, Contains and Overlaps compare
// boxes once they're placed; to them an Empty rect is no area, wherever it
// sits.
//
//	head, body := c.Frame().Inset(c.Unit(0.04), c.Unit(0.03)).CutTop(c.Y(0.18))
//	left, right := body.CutLeft(body.W * 0.4)
type Rect struct{ X, Y, W, H float64 }

// NewRect returns the w×h box with its top-left corner at (x, y), in pixels.
func NewRect(x, y, w, h float64) Rect { return Rect{X: x, Y: y, W: w, H: h} }

// Frame returns the whole canvas as a Rect.
func (c Ctx) Frame() Rect { return Rect{0, 0, c.PW(), c.PH()} }

// Rect returns the box at (fx, fy) of size fw×fh, all fractions of the canvas
// like X and Y.
func (c Ctx) Rect(fx, fy, fw, fh float64) Rect { return c.Frame().Sub(fx, fy, fw, fh) }

// Right returns the x of r's right edge.
func (r Rect) Right() float64 { return r.X + r.W }

// Bottom returns the y of r's bottom edge.
func (r Rect) Bottom() float64 { return r.Y + r.H }

// Center returns the point in the middle of r.
func (r Rect) Center() (x, y float64) { return r.X + r.W/2, r.Y + r.H/2 }

// Sub returns the box at (fx, fy) of size fw×fh, all fractions of r.
func (r Rect) Sub(fx, fy, fw, fh float64) Rect {
	return Rect{r.X + fx*r.W, r.Y + fy*r.H, fw * r.W, fh * r.H}
}

// Inset shrinks r by dx on the left and right and dy on the top and bottom;
// negative values grow it.
func (r Rect) Inset(dx, dy float64) Rect {
	return Rect{r.X + dx, r.Y + dy, max(r.W-2*dx, 0), max(r.H-2*dy, 0)}
}

// CutTop splits r into a strip h tall along its top and the rest below it.
func (r Rect) CutTop(h float64) (top, rest Rect) {
	h = min(max(h, 0), r.H)
	return Rect{r.X, r.Y, r.W, h}, Rect{r.X, r.Y + h, r.W, r.H - h}
}

// CutBottom splits r into a strip h tall along its bottom and the rest above.
func (r Rect) CutBottom(h float64) (bottom, rest Rect) {
	h = min(max(h, 0), r.H)
	return Rect{r.X, r.Bottom() - h, r.W, h}, Rect{r.X, r.Y, r.W, r.H - h}
}

// CutLeft splits r into a strip w wide along its left and the rest beside it.
func (r Rect) CutLeft(w float64) (left, rest Rect) {
	w = min(max(w, 0), r.W)
	return Rect{r.X, r.Y, w, r.H}, Rect{r.X + w, r.Y, r.W - w, r.H}
}

// CutRight splits r into a strip w wide along its right and the rest beside it.
func (r Rect) CutRight(w float64) (right, rest Rect) {
	w = min(max(w, 0), r.W)
	return Rect{r.Right() - w, r.Y, w, r.H}, Rect{r.X, r.Y, r.W - w, r.H}
}

// Rows splits r top to bottom into one row per weight, sized in proportion to
// the weights, with gap pixels between them. Rows(gap, 1, 1, 1) is three
// equal rows.
func (r Rect) Rows(gap float64, weights ...float64) []Rect {
	return spans(r.Y, r.H, gap, weights, func(y, h float64) Rect { return Rect{r.X, y, r.W, h} })
}

// Cols splits r left to right into one column per weight, as Rows does.
func (r Rect) Cols(gap float64, weights ...float64) []Rect {
	return spans(r.X, r.W, gap, weights, func(x, w float64) Rect { return Rect{x, r.Y, w, r.H} })
}

// spans shares length, less the gaps, among the weights, from start on; mk
// turns each span's start and size into a Rect.
func spans(start, length, gap float64, weights []float64, mk func(at, size float64) Rect) []Rect {
	sum := 0.0
	for _, w := range weights {
		sum += max(w, 0)
	}
	free := max(length-gap*float64(len(weights)-1), 0)
	out := make([]Rect, len(weights))
	for i, w := range weights {
		size := 0.0
		if sum > 0 {
			size = free * max(w, 0) / sum
		}
		out[i] = mk(start, size)
		start += size + gap
	}
	return out
}

// Grid splits r into cols×rows equal cells with gap pixels between them, in
// reading order: the cell at column i, row j is at index j*cols+i.
func (r Rect) Grid(cols, rows int, gap float64) []Rect {
	cols, rows = max(cols, 1), max(rows, 1)
	w := max((r.W-gap*float64(cols-1))/float64(cols), 0)
	h := max((r.H-gap*float64(rows-1))/float64(rows), 0)
	out := make([]Rect, 0, cols*rows)
	for j := range rows {
		for i := range cols {
			out = append(out, Rect{r.X + float64(i)*(w+gap), r.Y + float64(j)*(h+gap), w, h})
		}
	}
	return out
}

// Anchor returns a w×h box inside r, positioned by ax and ay: 0 against r's
// left or top edge, 0.5 centered, 1 against its right or bottom edge. A box
// bigger than r overhangs it by the same rule.
func (r Rect) Anchor(w, h, ax, ay float64) Rect {
	return Rect{r.X + (r.W-w)*ax, r.Y + (r.H-h)*ay, w, h}
}

// LerpRect moves a toward b: p=0 is a, p=1 is b.
func LerpRect(a, b Rect, p float64) Rect {
	return Rect{Lerp(a.X, b.X, p), Lerp(a.Y, b.Y, p), Lerp(a.W, b.W, p), Lerp(a.H, b.H, p)}
}

// Empty reports whether r has no area: a width or height that is zero,
// negative or NaN.
func (r Rect) Empty() bool { return !(r.W > 0 && r.H > 0) }

// Intersect returns the box covered by both r and o. When they share no area,
// because they only touch, don't meet or either is empty, it returns the zero
// Rect. A rect inside the other comes back unchanged.
func (r Rect) Intersect(o Rect) Rect {
	x, w, okx := spanOverlap(r.X, r.W, o.X, o.W)
	y, h, oky := spanOverlap(r.Y, r.H, o.Y, o.H)
	if !okx || !oky {
		return Rect{}
	}
	return Rect{x, y, w, h}
}

// Union returns the smallest box covering both r and o. An empty rect adds
// nothing: if o is empty Union returns r, else if r is empty it returns o. So
// the zero Rect is where to start gathering the bounds of several boxes.
func (r Rect) Union(o Rect) Rect {
	if o.Empty() {
		return r
	}
	if r.Empty() {
		return o
	}
	x, w := spanCover(r.X, r.W, o.X, o.W)
	y, h := spanCover(r.Y, r.H, o.Y, o.H)
	return Rect{x, y, w, h}
}

// Contains reports whether o lies inside r, edges included. An empty o is
// inside every rect, as the empty set is inside every set, so an empty r
// contains only empty rects.
func (r Rect) Contains(o Rect) bool {
	if o.Empty() {
		return true
	}
	return !r.Empty() && o.X >= r.X && o.Y >= r.Y && o.Right() <= r.Right() && o.Bottom() <= r.Bottom()
}

// Overlaps reports whether r and o share some area. Rects that only touch at
// an edge or a corner don't overlap, and an empty rect overlaps nothing: it is
// true exactly when Intersect returns a non-empty rect.
func (r Rect) Overlaps(o Rect) bool {
	return max(r.X, o.X) < min(r.Right(), o.Right()) && max(r.Y, o.Y) < min(r.Bottom(), o.Bottom())
}

// spanOverlap returns the start and size of the length the spans a..a+as and
// b..b+bs share; ok is false if they share none.
//
// A Rect keeps its size, not its far edge, and start+(end-start) can round to
// either side of end. So spanOverlap and spanCover keep a span's own size when
// the result is that span, and otherwise nudge the size an ulp at a time until
// the far edge lies inside both spans (an overlap) or reaches past both (a
// cover). Contains then agrees with what Intersect and Union build.
func spanOverlap(a, as, b, bs float64) (start, size float64, ok bool) {
	ae, be := a+as, b+bs
	start, end := max(a, b), min(ae, be)
	switch {
	case !(start < end):
		return 0, 0, false
	case start == a && end == ae:
		return a, as, true
	case start == b && end == be:
		return b, bs, true
	}
	size = end - start
	for start+size > end && size > 0 {
		size = math.Nextafter(size, 0)
	}
	return start, size, true
}

// spanCover returns the start and size of the shortest span covering both
// a..a+as and b..b+bs.
func spanCover(a, as, b, bs float64) (start, size float64) {
	ae, be := a+as, b+bs
	start, end := min(a, b), max(ae, be)
	switch {
	case start == a && end == ae:
		return a, as
	case start == b && end == be:
		return b, bs
	}
	size = end - start
	for start+size < end {
		size = math.Nextafter(size, math.Inf(1))
	}
	return start, size
}
