package decker

import (
	"math"
	"testing"
)

var (
	black = RGB{}
	white = RGB{255, 255, 255}
)

func blackCanvas(w, h int) *Pixels { return NewPixels(w, h, black) }

// maxDiff is the largest channel difference between two canvases of one size.
func maxDiff(a, b *Pixels) float64 {
	m := 0.0
	for i := range a.Pix {
		m = max(m, math.Abs(float64(a.Pix[i].R-b.Pix[i].R)))
	}
	return m
}

func TestPolygonSquareMatchesRect(t *testing.T) {
	for _, r := range [][4]float64{{10, 8, 30, 20}, {10.3, 8.6, 29.4, 19.2}} {
		x, y, w, h := r[0], r[1], r[2], r[3]
		a, b := blackCanvas(60, 40), blackCanvas(60, 40)
		a.Rect(x, y, w, h, white, 1)
		b.Polygon([]float64{x, y, x + w, y, x + w, y + h, x, y + h}, white, 1)
		// Edges match exactly; only the four corner pixels differ, where
		// distance coverage is rounder than area coverage.
		if d := maxDiff(a, b); d > 0.35*255 {
			t.Errorf("%v: polygon differs from Rect by %.0f of 255", r, d)
		}
		sum := func(p *Pixels) (s float64) {
			for _, c := range p.Pix {
				s += float64(c.R)
			}
			return s / 255
		}
		if want, got := sum(a), sum(b); math.Abs(want-got) > 1.5 {
			t.Errorf("%v: polygon covers %.2f px, Rect %.2f", r, got, want)
		}
	}
}

func TestPolygonConcaveNotchStaysEmpty(t *testing.T) {
	// A U: the notch between the arms is open at the top.
	u := []float64{10, 10, 20, 10, 20, 30, 30, 30, 30, 10, 40, 10, 40, 40, 10, 40}
	p := blackCanvas(50, 50)
	p.Polygon(u, white, 1)
	for _, c := range [][2]int{{25, 15}, {25, 25}, {22, 12}} {
		if got := p.At(c[0], c[1]); got != black {
			t.Errorf("notch pixel %v = %v, want empty", c, got)
		}
	}
	for _, c := range [][2]int{{14, 25}, {35, 25}, {25, 35}, {15, 12}} {
		if got := p.At(c[0], c[1]); got != white {
			t.Errorf("arm pixel %v = %v, want filled", c, got)
		}
	}
}

func TestPolygonEvenOddLeavesStarHollow(t *testing.T) {
	var pts []float64
	for i := 0; i < 5; i++ { // a pentagram, drawn as one self-crossing outline
		a := float64(i) * 4 * math.Pi / 5
		pts = append(pts, 30+25*math.Sin(a), 30-25*math.Cos(a))
	}
	p := blackCanvas(60, 60)
	p.Polygon(pts, white, 1)
	if got := p.At(30, 30); got != black {
		t.Errorf("pentagram center = %v, want a hole", got)
	}
	if got := p.At(30, 8); got != white {
		t.Errorf("pentagram tip = %v, want filled", got)
	}
}

func TestPolygonEdgeCases(t *testing.T) {
	p := blackCanvas(20, 20)
	p.Polygon([]float64{1, 1, 5, 5}, white, 1)    // two points
	p.Polygon([]float64{1, 1, 5, 5, 9}, white, 1) // dangling coordinate
	p.Polygon([]float64{1, 1, 9, 1, 9, 9}, white, 0)
	p.Polygon([]float64{-50, -50, 5, -50, 5, -40}, white, 1) // off canvas
	for _, c := range p.Pix {
		if c != black {
			t.Fatal("degenerate polygons drew something")
		}
	}
}

func TestEllipseCircleMatchesDisc(t *testing.T) {
	for _, r := range []float64{4.5, 12, 20.25} {
		a, b := blackCanvas(60, 60), blackCanvas(60, 60)
		a.Disc(30.3, 29.6, r, white, 1)
		b.Ellipse(30.3, 29.6, r, r, 0, white, 1)
		if d := maxDiff(a, b); d > 1 {
			t.Errorf("r=%v: ellipse differs from Disc by %.2f", r, d)
		}
		a, b = blackCanvas(60, 60), blackCanvas(60, 60)
		a.RoundRect(30.3-r, 29.6-r, 2*r, 2*r, r, 2, white, 0.7)
		b.Ellipse(30.3, 29.6, r, r, 2, white, 0.7)
		if d := maxDiff(a, b); d > 1 {
			t.Errorf("r=%v: outline differs from RoundRect ring by %.2f", r, d)
		}
	}
}

func TestEllipseShape(t *testing.T) {
	p := blackCanvas(80, 40)
	p.Ellipse(40.5, 20, 30, 10, 0, white, 1)
	for _, c := range [][2]int{{40, 20}, {12, 20}, {67, 20}, {40, 11}, {40, 28}} {
		if p.At(c[0], c[1]) != white {
			t.Errorf("%v should be inside", c)
		}
	}
	for _, c := range [][2]int{{5, 20}, {40, 5}, {40, 35}, {15, 12}, {65, 28}} {
		if p.At(c[0], c[1]) != black {
			t.Errorf("%v should be outside", c)
		}
	}
	// The edge is within half a pixel of where the radii put it.
	if got := p.At(70, 20).R; got < 100 || got > 200 {
		t.Errorf("edge pixel at the x radius = %v, want partial coverage", got)
	}
	o := blackCanvas(80, 40)
	o.Ellipse(40, 20, 30, 10, 3, white, 1)
	if o.At(40, 20) != black || o.At(40, 11).R == 0 {
		t.Error("outline should be hollow with a ring at the edge")
	}
	o.Ellipse(40, 20, 0, 10, 0, white, 1) // no radius: nothing
}

func TestPolylineJointsDontDoubleBlend(t *testing.T) {
	pts := []float64{10, 10, 40, 30, 10, 32, 45, 12}
	p := blackCanvas(60, 50)
	p.Polyline(pts, 6, white, 0.5)
	hi := 0.0
	for _, c := range p.Pix {
		hi = max(hi, float64(c.R))
	}
	if want := 0.5 * 255; hi > want+0.5 {
		t.Errorf("brightest pixel = %.1f, want at most %.1f: segments blended twice", hi, want)
	}
	// Separate Line calls do double-blend at the shared joint, which is what
	// Polyline avoids.
	q := blackCanvas(60, 50)
	for i := 0; i+3 < len(pts); i += 2 {
		q.Line(pts[i], pts[i+1], pts[i+2], pts[i+3], 6, white, 0.5)
	}
	hi = 0
	for _, c := range q.Pix {
		hi = max(hi, float64(c.R))
	}
	if hi <= 0.5*255+1 {
		t.Skip("Line calls did not overlap in this geometry")
	}
}

func TestPolylineMatchesLineForOneSegment(t *testing.T) {
	a, b := blackCanvas(50, 30), blackCanvas(50, 30)
	a.Line(5, 6, 44, 20, 3, white, 0.8)
	b.Polyline([]float64{5, 6, 44, 20}, 3, white, 0.8)
	if d := maxDiff(a, b); d > 1 {
		t.Errorf("polyline differs from Line by %.2f", d)
	}
	b = blackCanvas(50, 30)
	b.Polyline([]float64{25, 15}, 6, white, 1) // one point is a dot
	if b.At(25, 15) != white || b.At(25, 25) != black {
		t.Error("a single point should draw a dot")
	}
	b.Polyline(nil, 6, white, 1)
}

func TestBezierEndpointsAndProg(t *testing.T) {
	const x0, y0, x1, y1 = 10.0, 40.0, 110.0, 40.0
	draw := func(prog float64) *Pixels {
		p := blackCanvas(120, 60)
		p.Bezier(x0, y0, 40, 5, 80, 5, x1, y1, 3, white, 1, prog)
		return p
	}
	full := draw(1)
	if full.At(10, 40).R == 0 || full.At(109, 40).R == 0 {
		t.Error("a full curve should reach both endpoints")
	}
	// The curve at t=0.5 is at (60, 13.75).
	if full.At(60, 13).R == 0 && full.At(60, 14).R == 0 {
		t.Error("the curve should pass through its midpoint")
	}
	if full.At(60, 40) != black {
		t.Error("the chord should be empty: the curve bows away from it")
	}
	if got := draw(0); maxDiff(got, blackCanvas(120, 60)) != 0 {
		t.Error("prog 0 should draw nothing")
	}
	half := draw(0.5)
	if half.At(10, 40).R == 0 || half.At(60, 13).R+half.At(60, 14).R == 0 {
		t.Error("half a symmetric curve should reach its middle")
	}
	if half.At(109, 40) != black || half.At(95, 30) != black {
		t.Error("half a curve should leave its far end empty")
	}
	// Arc length, not t: prog covers that fraction of the length, within the
	// flattening error.
	pts := flattenBezier(nil, x0, y0, 40, 5, 80, 5, x1, y1)
	total := pathLen(pts)
	part := pathSpan(nil, pts, 0, 0.25*total)
	if got := pathLen(part); math.Abs(got-0.25*total) > 1e-6 {
		t.Errorf("quarter span is %.4f long, want %.4f", got, 0.25*total)
	}
	if got := draw(1.7); maxDiff(got, full) != 0 {
		t.Error("prog above 1 should clamp to the whole curve")
	}
}

func TestDashedLinePattern(t *testing.T) {
	p := blackCanvas(100, 10)
	p.DashedLine(0, 5, 100, 5, 2, 10, 10, white, 1)
	// Dashes cover 0-10, 20-30, ...; round caps spill 1px into each gap.
	for _, x := range []int{5, 25, 45, 85} {
		if p.At(x, 5).R == 0 {
			t.Errorf("x=%d should be in a dash", x)
		}
	}
	for _, x := range []int{15, 35, 55, 95} {
		if p.At(x, 5) != black {
			t.Errorf("x=%d should be in a gap", x)
		}
	}
	q := blackCanvas(100, 10)
	q.DashedLine(0, 5, 100, 5, 2, 0, 10, white, 1) // no dash: solid
	if q.At(15, 5).R == 0 {
		t.Error("a zero dash should draw a solid line")
	}
	q.DashedLine(3, 3, 3, 3, 2, 5, 5, white, 1) // zero length
}

func TestDashedPathFollowsCorners(t *testing.T) {
	p := blackCanvas(60, 60)
	p.dashedPath([]float64{5, 5, 45, 5, 45, 45}, 2, 8, 6, white, 1)
	if p.At(8, 5).R == 0 || p.At(45, 20).R == 0 && p.At(45, 21).R == 0 && p.At(45, 22).R == 0 {
		t.Error("dashes should run along both legs")
	}
	if p.At(15, 5) != black {
		t.Error("the first gap should be empty")
	}
}

func TestGradients(t *testing.T) {
	a, b := RGB{255, 0, 0}, RGB{0, 0, 255}
	p := blackCanvas(100, 40)
	p.LinearGradient(10, 5, 80, 30, 8, 10, 0, 90, 0, a, b, 1)
	l, m, r := p.At(14, 20), p.At(50, 20), p.At(85, 20)
	if !(l.R > m.R && m.R > r.R && l.B < m.B && m.B < r.B) {
		t.Errorf("gradient should run red to blue: %v %v %v", l, m, r)
	}
	if p.At(10, 5) != black {
		t.Error("the rounded corner should be clear")
	}
	if p.At(5, 20) != black {
		t.Error("nothing outside the rect")
	}
	q := blackCanvas(100, 40)
	q.RadialGradient(10, 5, 80, 30, 0, 50, 20, 30, a, b, 1)
	if c := q.At(50, 20); c.R < 240 {
		t.Errorf("center = %v, want the inner color", c)
	}
	if c := q.At(12, 6); c.B < 240 {
		t.Errorf("far corner = %v, want the outer color", c)
	}
	p.LinearGradient(0, 0, 10, 10, 0, 1, 1, 1, 1, a, b, 1) // zero-length axis
	q.RadialGradient(0, 0, 10, 10, 0, 1, 1, 0, a, b, 1)    // zero radius
}

func TestShapesOffCanvasAndTiny(t *testing.T) {
	p := blackCanvas(20, 20)
	p.Polyline([]float64{-100, -100, -50, -80}, 3, white, 1)
	p.Bezier(-50, -50, -30, 0, -10, 0, -5, -5, 2, white, 1, 1)
	p.Ellipse(-40, -40, 5, 3, 0, white, 1)
	p.Polyline([]float64{1, 1, 5, 5}, 0, white, 1) // no width
	for _, c := range p.Pix {
		if c != black {
			t.Fatal("off-canvas or zero-width shapes drew something")
		}
	}
	p.Polyline([]float64{-5, 10, 30, 10}, 2, white, 1) // clipped at both sides
	if p.At(0, 10).R == 0 || p.At(19, 10).R == 0 {
		t.Error("a clipped line should still cross the canvas")
	}
}

func TestShapesDontAllocate(t *testing.T) {
	p := blackCanvas(200, 100)
	pts := []float64{10, 10, 100, 80, 20, 90, 150, 20}
	allocs := testing.AllocsPerRun(20, func() {
		p.Polyline(pts, 3, white, 0.5)
		p.Bezier(10, 80, 60, 5, 120, 5, 190, 80, 3, white, 1, 0.8)
		p.Polygon(pts, white, 0.5)
		p.dashedPath(pts, 2, 6, 4, white, 1)
	})
	if allocs > 0 {
		t.Errorf("shapes allocated %v times per run", allocs)
	}
}
