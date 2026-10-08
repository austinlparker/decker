package decker

// Rect is a box on the pixel canvas: X, Y is its top-left corner and W, H its
// size, all in pixels. Layout is cutting rects out of rects: start from
// Ctx.Frame or Ctx.Rect, then Inset, Cut, Rows, Cols, Grid or Anchor, and hand the
// pieces to whatever draws there. Every method returns new rects; widths and
// heights never go below zero.
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
