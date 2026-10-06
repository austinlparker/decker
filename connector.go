package decker

import "math"

// Route is the shape of a Connector's path.
type Route int

// The routes a Connector can take.
const (
	RouteStraight Route = iota // one straight line between the two anchors
	RouteElbow                 // axis-aligned segments with one or two bends
	RouteCurved                // a cubic Bezier leaving and arriving square to the sides
)

// ArrowHead is the mark at one end of a Connector.
type ArrowHead int

// The marks a Connector's ends can carry.
const (
	HeadNone  ArrowHead = iota // a bare end
	HeadArrow                  // a filled triangle whose tip touches the rect's edge
	HeadDot                    // a disc resting against the rect's edge
)

// Side names a side of a Rect.
type Side int

// The sides of a Rect. SideAuto, the zero value, lets the Connector choose.
const (
	SideAuto Side = iota
	SideLeft
	SideRight
	SideTop
	SideBottom
)

// anchor returns the middle of side s of r.
func (s Side) anchor(r Rect) (x, y float64) {
	cx, cy := r.Center()
	switch s {
	case SideLeft:
		return r.X, cy
	case SideRight:
		return r.Right(), cy
	case SideTop:
		return cx, r.Y
	}
	return cx, r.Bottom()
}

// normal returns the unit vector pointing out of a rect through side s.
func (s Side) normal() (x, y float64) {
	switch s {
	case SideLeft:
		return -1, 0
	case SideRight:
		return 1, 0
	case SideTop:
		return 0, -1
	}
	return 0, 1
}

// Connector joins one Rect to another with a line, an elbow or a curve,
// optionally capped with arrowheads and labeled. It leaves From and arrives at
// To, at the middle of the sides that face each other; set FromSide or ToSide
// to pick a side instead. The arrowhead's tip sits exactly on the target's
// edge.
//
//	Connector{From: a, To: b, Route: RouteElbow, Head: HeadArrow, Prog: Ease(c.Since(1), 0.6)}.Draw(c, p)
type Connector struct {
	From, To Rect
	// FromSide and ToSide force the side the path leaves and arrives at. If
	// the sides picked don't face each other, an elbow or a line can run
	// through the rects.
	FromSide, ToSide Side
	Route            Route
	// Head marks the To end and Tail the From end.
	Head, Tail ArrowHead
	Width      float64 // stroke width; 0 uses a width that scales with the canvas
	Color      RGB     // zero uses the theme's Muted
	Dashed     bool
	Label      string // on a plate at the path's midpoint
	// Prog is how much of the path is drawn, 0..1, by length from the From end;
	// the label fades in at the end. 0 draws nothing, so a connector that is
	// always there wants 1. Animate it with Ease(c.Since(step), dur).
	Prog float64
}

// autoSides picks the sides of a and b that face each other: left and right
// when they are apart more across than down, top and bottom otherwise. Rects
// that overlap on both axes go by the offset of their centers.
func autoSides(a, b Rect) (from, to Side) {
	gapX := max(b.X-a.Right(), a.X-b.Right())
	gapY := max(b.Y-a.Bottom(), a.Y-b.Bottom())
	acx, acy := a.Center()
	bcx, bcy := b.Center()
	horizontal := gapX >= gapY
	if gapX <= 0 && gapY <= 0 {
		horizontal = math.Abs(bcx-acx) >= math.Abs(bcy-acy)
	}
	if horizontal {
		if bcx >= acx {
			return SideRight, SideLeft
		}
		return SideLeft, SideRight
	}
	if bcy >= acy {
		return SideBottom, SideTop
	}
	return SideTop, SideBottom
}

// sides resolves the sides the connector leaves and arrives at. A side left
// on auto faces the other end: when both are auto, rects that sit diagonally
// apart get an elbow with a single bend, leaving along the longer gap and
// arriving across the shorter.
func (k Connector) sides() (from, to Side) {
	from, to = k.FromSide, k.ToSide
	switch {
	case from == SideAuto && to == SideAuto:
		from, to = autoSides(k.From, k.To)
		gapX := max(k.To.X-k.From.Right(), k.From.X-k.To.Right())
		gapY := max(k.To.Y-k.From.Bottom(), k.From.Y-k.To.Bottom())
		if k.Route == RouteElbow && gapX > 0 && gapY > 0 {
			acx, acy := k.From.Center()
			bcx, bcy := k.To.Center()
			switch {
			case gapX >= gapY && bcy >= acy:
				to = SideTop
			case gapX >= gapY:
				to = SideBottom
			case bcx >= acx:
				to = SideLeft
			default:
				to = SideRight
			}
		}
	case from == SideAuto:
		x, y := to.anchor(k.To)
		from = facing(k.From, x, y)
	case to == SideAuto:
		x, y := from.anchor(k.From)
		to = facing(k.To, x, y)
	}
	return from, to
}

// facing is the side of r toward the point (x, y): the axis along which the
// point is further, measured in widths and heights of r, so a point beside a
// wide rect is still above it.
func facing(r Rect, x, y float64) Side {
	cx, cy := r.Center()
	dx, dy := (x-cx)/max(r.W, 1e-9), (y-cy)/max(r.H, 1e-9)
	if math.Abs(dx) >= math.Abs(dy) {
		if dx >= 0 {
			return SideRight
		}
		return SideLeft
	}
	if dy >= 0 {
		return SideBottom
	}
	return SideTop
}

// path appends the polyline from the From anchor to the To anchor, curves
// flattened, and returns it.
func (k Connector) path(dst []float64, margin float64) []float64 {
	fs, ts := k.sides()
	ax, ay := fs.anchor(k.From)
	bx, by := ts.anchor(k.To)
	fnx, fny := fs.normal()
	tnx, tny := ts.normal()
	switch k.Route {
	case RouteCurved:
		d := math.Hypot(bx-ax, by-ay) * 0.45
		return flattenBezier(dst, ax, ay, ax+fnx*d, ay+fny*d, bx+tnx*d, by+tny*d, bx, by)
	case RouteElbow:
		return elbow(dst, ax, ay, fnx, fny, bx, by, tnx, tny, margin)
	}
	return append(dst, ax, ay, bx, by)
}

// elbow appends the orthogonal path from A, leaving along (fnx, fny), to B,
// arriving against (tnx, tny): one bend between a horizontal and a vertical
// side, two otherwise. Opposite sides meet halfway across; the same side
// swings out margin past both anchors. A path that needs no bend is a line.
func elbow(dst []float64, ax, ay, fnx, fny, bx, by, tnx, tny, margin float64) []float64 {
	const eps = 1e-6
	fHoriz, tHoriz := fny == 0, tny == 0
	switch {
	case fHoriz && tHoriz:
		if math.Abs(ay-by) < eps && fnx*tnx < 0 {
			return append(dst, ax, ay, bx, by)
		}
		mx := (ax + bx) / 2
		if fnx*tnx > 0 { // both leave the same way: C shape
			mx = max(ax, bx) + margin
			if fnx < 0 {
				mx = min(ax, bx) - margin
			}
		}
		return append(dst, ax, ay, mx, ay, mx, by, bx, by)
	case !fHoriz && !tHoriz:
		if math.Abs(ax-bx) < eps && fny*tny < 0 {
			return append(dst, ax, ay, bx, by)
		}
		my := (ay + by) / 2
		if fny*tny > 0 {
			my = max(ay, by) + margin
			if fny < 0 {
				my = min(ay, by) - margin
			}
		}
		return append(dst, ax, ay, ax, my, bx, my, bx, by)
	case fHoriz:
		return append(dst, ax, ay, bx, ay, bx, by)
	}
	return append(dst, ax, ay, ax, by, bx, by)
}

// headSize is the length of an arrowhead for a stroke of width w. The tip's
// half-angle is Arrow's, 0.5 radians, so a connector's heads are shaped like
// Arrow's.
func headSize(h ArrowHead, w float64) float64 {
	switch h {
	case HeadArrow:
		return w * 5 * math.Cos(0.5)
	case HeadDot:
		return w * 1.8
	}
	return 0
}

// drawHead draws the mark h with its tip at (tx, ty), pointing along the unit
// vector (dx, dy), at scale f (1 is full size).
func drawHead(p *Pixels, h ArrowHead, tx, ty, dx, dy, w, f float64, col RGB) {
	l := headSize(h, w) * f
	switch h {
	case HeadArrow:
		half := w * 5 * math.Sin(0.5) * f
		bx, by := tx-dx*l, ty-dy*l
		p.Polygon([]float64{tx, ty, bx - dy*half, by + dx*half, bx + dy*half, by - dx*half}, col, 1)
	case HeadDot:
		p.Disc(tx-dx*l, ty-dy*l, l, col, 1)
	}
}

// Draw renders the connector on p.
func (k Connector) Draw(c Ctx, p *Pixels) {
	prog := min(k.Prog, 1)
	if prog <= 0 {
		return
	}
	width := k.Width
	if width <= 0 {
		width = max(c.Unit(0.008), 1.5)
	}
	col := k.Color
	if col == (RGB{}) {
		col = c.Theme.Muted
	}
	fp, bp := getPts(), getPts()
	defer putPts(fp)
	defer putPts(bp)
	*fp = k.path(*fp, c.Unit(0.05))
	full := *fp
	total := pathLen(full)
	if total == 0 {
		return
	}

	// The stroke runs between the arrowheads' bases, so a head never has a
	// line poking out of its tip. Short paths shrink the heads to fit.
	tl, hl := headSize(k.Tail, width), headSize(k.Head, width)
	scale := min(1, total*0.8/max(tl+hl, 1e-9))
	tl, hl = tl*scale, hl*scale
	cut := prog * total
	if s1 := min(cut, total-hl); s1 > tl {
		*bp = pathSpan(*bp, full, tl, s1)
		if k.Dashed {
			p.dashedPath(*bp, width, max(width*4, c.Unit(0.02)), max(width*2.5, c.Unit(0.012)), col, 1)
		} else {
			p.stroke(*bp, width, col, 1)
		}
	}
	if tl > 0 {
		sx, sy := full[0], full[1]
		px, py := pathAt(full, tl)
		dx, dy := unit(sx-px, sy-py)
		drawHead(p, k.Tail, sx, sy, dx, dy, width, scale*Clamp01(cut/tl), col)
	}
	if hl > 0 && cut > total-hl {
		ex, ey := full[len(full)-2], full[len(full)-1]
		px, py := pathAt(full, total-hl)
		dx, dy := unit(ex-px, ey-py)
		drawHead(p, k.Head, ex, ey, dx, dy, width, scale*Clamp01((cut-(total-hl))/hl), col)
	}
	if k.Label != "" && prog > 0.5 {
		mx, my := pathAt(full, total/2)
		LineLabel(c, p, c.Theme.Body, k.Label, mx, my, col, (prog-0.5)*2)
	}
}

// unit returns (x, y) scaled to length 1, or (1, 0) for a zero vector.
func unit(x, y float64) (float64, float64) {
	l := math.Hypot(x, y)
	if l == 0 {
		return 1, 0
	}
	return x / l, y / l
}
