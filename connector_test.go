package decker

import (
	"math"
	"testing"
)

func TestConnectorSideSelection(t *testing.T) {
	a := Rect{100, 100, 40, 20}
	for name, tc := range map[string]struct {
		to       Rect
		from, at Side
	}{
		"right":         {Rect{200, 100, 40, 20}, SideRight, SideLeft},
		"left":          {Rect{0, 100, 40, 20}, SideLeft, SideRight},
		"below":         {Rect{100, 200, 40, 20}, SideBottom, SideTop},
		"above":         {Rect{100, 0, 40, 20}, SideTop, SideBottom},
		"right, lower":  {Rect{200, 130, 40, 20}, SideRight, SideLeft},
		"below, right":  {Rect{120, 200, 40, 20}, SideBottom, SideTop},
		"overlap, tall": {Rect{110, 110, 40, 100}, SideBottom, SideTop},
	} {
		for _, route := range []Route{RouteStraight, RouteCurved} {
			from, to := Connector{From: a, To: tc.to, Route: route}.sides()
			if from != tc.from || to != tc.at {
				t.Errorf("%s (route %d): sides = %d, %d, want %d, %d", name, route, from, to, tc.from, tc.at)
			}
		}
	}
}

func TestConnectorSideOverrides(t *testing.T) {
	a, b := Rect{100, 100, 40, 20}, Rect{200, 100, 40, 20}
	from, to := Connector{From: a, To: b, FromSide: SideTop, ToSide: SideTop}.sides()
	if from != SideTop || to != SideTop {
		t.Errorf("both overridden: %d, %d", from, to)
	}
	// One side forced: the other end faces it.
	from, to = Connector{From: a, To: b, FromSide: SideBottom}.sides()
	if from != SideBottom || to != SideLeft {
		t.Errorf("From forced to the bottom: %d, %d, want To's left, which faces the anchor", from, to)
	}
	from, to = Connector{From: a, To: b, ToSide: SideLeft}.sides()
	if from != SideRight || to != SideLeft {
		t.Errorf("To forced to the left: %d, %d", from, to)
	}
}

func TestConnectorElbowSides(t *testing.T) {
	a := Rect{0, 0, 40, 20}
	// Diagonal rects get one bend: out along the longer gap, in across the shorter.
	from, to := Connector{From: a, To: Rect{200, 80, 40, 20}, Route: RouteElbow}.sides()
	if from != SideRight || to != SideTop {
		t.Errorf("diagonal, wider gap: %d, %d", from, to)
	}
	from, to = Connector{From: a, To: Rect{60, 200, 40, 20}, Route: RouteElbow}.sides()
	if from != SideBottom || to != SideLeft {
		t.Errorf("diagonal, taller gap: %d, %d", from, to)
	}
	from, to = Connector{From: Rect{200, 200, 40, 20}, To: a, Route: RouteElbow}.sides()
	if from != SideLeft && from != SideTop || to != SideBottom && to != SideRight {
		t.Errorf("diagonal up-left: %d, %d", from, to)
	}
}

func orthogonal(pts []float64) bool {
	for i := 2; i+1 < len(pts); i += 2 {
		if pts[i] != pts[i-2] && pts[i+1] != pts[i-1] {
			return false
		}
	}
	return true
}

func TestConnectorElbowRouting(t *testing.T) {
	a := Rect{0, 0, 40, 20}
	for name, tc := range map[string]struct {
		k     Connector
		bends int
		start [2]float64
		end   [2]float64
	}{
		"level":     {Connector{From: a, To: Rect{100, 0, 40, 20}}, 0, [2]float64{40, 10}, [2]float64{100, 10}},
		"offset":    {Connector{From: a, To: Rect{100, 5, 40, 20}}, 2, [2]float64{40, 10}, [2]float64{100, 15}},
		"stacked":   {Connector{From: a, To: Rect{10, 80, 40, 20}}, 2, [2]float64{20, 20}, [2]float64{30, 80}},
		"diagonal":  {Connector{From: a, To: Rect{100, 80, 40, 20}}, 1, [2]float64{40, 10}, [2]float64{120, 80}},
		"same side": {Connector{From: a, To: Rect{100, 50, 40, 20}, FromSide: SideRight, ToSide: SideRight}, 2, [2]float64{40, 10}, [2]float64{140, 60}},
		"top-top":   {Connector{From: a, To: Rect{100, 50, 40, 20}, FromSide: SideTop, ToSide: SideTop}, 2, [2]float64{20, 0}, [2]float64{120, 50}},
	} {
		tc.k.Route = RouteElbow
		pts := tc.k.path(nil, 12)
		if got := len(pts)/2 - 2; got != tc.bends {
			t.Errorf("%s: %d bends in %v, want %d", name, got, pts, tc.bends)
		}
		if !orthogonal(pts) {
			t.Errorf("%s: path %v is not orthogonal", name, pts)
		}
		if pts[0] != tc.start[0] || pts[1] != tc.start[1] || pts[len(pts)-2] != tc.end[0] || pts[len(pts)-1] != tc.end[1] {
			t.Errorf("%s: path %v should run %v to %v", name, pts, tc.start, tc.end)
		}
	}
	// A Z bend turns halfway across the gap.
	pts := Connector{From: a, To: Rect{100, 5, 40, 20}, Route: RouteElbow}.path(nil, 12)
	if pts[2] != 70 || pts[4] != 70 {
		t.Errorf("Z bends at x=%v, %v, want 70", pts[2], pts[4])
	}
	// A C bend swings out past both rects by the margin.
	pts = Connector{From: a, To: Rect{100, 50, 40, 20}, Route: RouteElbow, FromSide: SideRight, ToSide: SideRight}.path(nil, 12)
	if pts[2] != 152 {
		t.Errorf("C bends at x=%v, want 152", pts[2])
	}
}

func TestConnectorCurveLeavesSquare(t *testing.T) {
	k := Connector{From: Rect{0, 0, 40, 20}, To: Rect{200, 100, 40, 20}, Route: RouteCurved}
	pts := k.path(nil, 12)
	n := len(pts)
	if pts[0] != 40 || pts[1] != 10 || pts[n-2] != 200 || pts[n-1] != 110 {
		t.Errorf("curve runs from (%v, %v) to (%v, %v)", pts[0], pts[1], pts[n-2], pts[n-1])
	}
	// The tangent at each end is along the side's normal: the first and last
	// steps are nearly horizontal.
	if dy, dx := pts[3]-pts[1], pts[2]-pts[0]; math.Abs(dy) > 0.1*dx {
		t.Errorf("curve leaves at slope %.3f, want flat", dy/dx)
	}
	if dy, dx := pts[n-1]-pts[n-3], pts[n-2]-pts[n-4]; math.Abs(dy) > 0.1*dx {
		t.Errorf("curve arrives at slope %.3f, want flat", dy/dx)
	}
}

func connectorCtx() Ctx { return Ctx{W: 120, H: 40, Theme: testTheme, T: 1, StepT: 1} }

func TestConnectorArrowheadIsFlush(t *testing.T) {
	c := connectorCtx()
	to := Rect{90, 30, 20, 20}
	for _, route := range []Route{RouteStraight, RouteElbow, RouteCurved} {
		p := NewPixels(120, 80, black)
		Connector{From: Rect{10, 30, 20, 20}, To: to, Route: route, Head: HeadArrow, Width: 2, Color: white, Prog: 1}.Draw(c, p)
		// Ink right up to the target's edge, none inside the rect.
		if p.At(89, 40).R == 0 {
			t.Errorf("route %d: no ink at the target's edge", route)
		}
		for y := 30; y < 50; y++ {
			for x := 90; x < 110; x++ {
				if p.At(x, y) != black {
					t.Fatalf("route %d: ink inside the target at (%d, %d)", route, x, y)
				}
			}
		}
		// The head is wider than the stroke.
		wide := 0
		for y := 0; y < 80; y++ {
			if p.At(84, y).R > 40 {
				wide++
			}
		}
		if wide < 4 {
			t.Errorf("route %d: head is %d px wide at x=84, want a triangle wider than the stroke", route, wide)
		}
	}
}

func TestConnectorProgDrawsOn(t *testing.T) {
	c := connectorCtx()
	k := Connector{From: Rect{10, 30, 20, 20}, To: Rect{90, 30, 20, 20}, Head: HeadArrow, Tail: HeadDot, Width: 2, Color: white, Label: "go"}
	draw := func(prog float64) *Pixels {
		p := NewPixels(120, 80, black)
		k.Prog = prog
		k.Draw(c, p)
		return p
	}
	if got := draw(0); maxDiff(got, NewPixels(120, 80, black)) != 0 {
		t.Error("prog 0 should draw nothing")
	}
	half := draw(0.5)
	if half.At(35, 40).R == 0 {
		t.Error("half the connector should have drawn from the start")
	}
	if half.At(80, 40) != black || half.At(88, 40) != black {
		t.Error("half the connector should not reach the target")
	}
	full := draw(1)
	if full.At(88, 40).R == 0 {
		t.Error("the full connector should reach the target")
	}
	if full.At(60, 36) == black && full.At(60, 44) == black {
		t.Error("the label plate should cover the midpoint")
	}
	prev := 0.0
	for _, prog := range []float64{0.1, 0.3, 0.5, 0.7, 0.9, 1} {
		sum := 0.0
		for _, px := range draw(prog).Pix {
			sum += float64(px.R)
		}
		if sum < prev {
			t.Errorf("prog %.1f drew less than the one before", prog)
		}
		prev = sum
	}
}

func TestConnectorDrawsEveryStyle(t *testing.T) {
	c := connectorCtx()
	a, b := Rect{10, 10, 20, 12}, Rect{80, 50, 20, 12}
	for _, route := range []Route{RouteStraight, RouteElbow, RouteCurved} {
		for _, dashed := range []bool{false, true} {
			for _, h := range []ArrowHead{HeadNone, HeadArrow, HeadDot} {
				p := NewPixels(120, 80, black)
				Connector{From: a, To: b, Route: route, Head: h, Tail: h, Dashed: dashed, Prog: 0.7}.Draw(c, p)
				// Overlapping rects and a zero-size one must not panic either.
				Connector{From: a, To: a, Route: route, Head: h, Prog: 1}.Draw(c, p)
				Connector{From: Rect{5, 5, 0, 0}, To: Rect{5, 5, 0, 0}, Route: route, Head: h, Tail: h, Prog: 1, Label: "x"}.Draw(c, p)
			}
		}
	}
}

func TestConnectorDefaultsFollowTheTheme(t *testing.T) {
	c := connectorCtx()
	p := NewPixels(120, 80, black)
	Connector{From: Rect{10, 30, 20, 20}, To: Rect{90, 30, 20, 20}, Prog: 1}.Draw(c, p)
	if got := p.At(60, 40); got != testTheme.Muted && got.R < 100 {
		t.Errorf("default color = %v, want the theme's Muted", got)
	}
}
