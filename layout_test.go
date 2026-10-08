package decker

import (
	"math"
	"testing"
)

func TestRectCuts(t *testing.T) {
	r := Rect{10, 20, 100, 50}
	if top, rest := r.CutTop(15); top != (Rect{10, 20, 100, 15}) || rest != (Rect{10, 35, 100, 35}) {
		t.Errorf("CutTop: %v %v", top, rest)
	}
	if bot, rest := r.CutBottom(80); bot != r || rest != (Rect{10, 20, 100, 0}) {
		t.Errorf("CutBottom past the edge: %v %v", bot, rest)
	}
	if left, rest := r.CutLeft(-5); left != (Rect{10, 20, 0, 50}) || rest != r {
		t.Errorf("CutLeft negative: %v %v", left, rest)
	}
	if right, rest := r.CutRight(30); right != (Rect{80, 20, 30, 50}) || rest != (Rect{10, 20, 70, 50}) {
		t.Errorf("CutRight: %v %v", right, rest)
	}
	if got := r.Inset(60, 5); got != (Rect{70, 25, 0, 40}) {
		t.Errorf("Inset past zero: %v", got)
	}
}

func TestRectSplits(t *testing.T) {
	r := Rect{0, 0, 110, 40}
	cols := r.Cols(10, 1, 3, 0, -1)
	want := []Rect{{0, 0, 20, 40}, {30, 0, 60, 40}, {100, 0, 0, 40}, {110, 0, 0, 40}}
	for i := range want {
		if cols[i] != want[i] {
			t.Errorf("Cols[%d] = %v, want %v", i, cols[i], want[i])
		}
	}
	if rows := r.Rows(0); len(rows) != 0 {
		t.Errorf("Rows with no weights: %v", rows)
	}
	if rows := r.Rows(4, 0, 0); rows[0].H != 0 || rows[1].Y != 4 {
		t.Errorf("Rows with zero weights: %v", rows)
	}
	g := r.Grid(2, 2, 10)
	if len(g) != 4 || g[3] != (Rect{60, 25, 50, 15}) {
		t.Errorf("Grid: %v", g)
	}
	if g := r.Grid(0, 0, 0); len(g) != 1 || g[0] != r {
		t.Errorf("Grid of zero: %v", g)
	}
}

func TestRectPlace(t *testing.T) {
	r := Rect{0, 0, 100, 50}
	if got := r.Anchor(20, 10, 1, 0.5); got != (Rect{80, 20, 20, 10}) {
		t.Errorf("Place: %v", got)
	}
	if x, y := r.Center(); x != 50 || y != 25 {
		t.Errorf("Center: %v %v", x, y)
	}
	if got := NewRect(1, 2, 3, 4); got != (Rect{1, 2, 3, 4}) {
		t.Errorf("NewRect: %v", got)
	}
	if got := LerpRect(r, Rect{100, 50, 0, 0}, 0.5); got != (Rect{50, 25, 50, 25}) {
		t.Errorf("LerpRect: %v", got)
	}
	c := Ctx{W: 200, H: 50}
	if got := c.Rect(0.5, 0.5, 0.25, 0.1); got != (Rect{100, 50, 50, 10}) {
		t.Errorf("Ctx.Rect: %v", got)
	}
}

func TestRectIntersectAndOverlaps(t *testing.T) {
	r := Rect{0, 0, 100, 50}
	for _, tc := range []struct {
		o    Rect
		want Rect
	}{
		{Rect{60, 30, 100, 100}, Rect{60, 30, 40, 20}},
		{Rect{10, 10, 20, 20}, Rect{10, 10, 20, 20}}, // inside: unchanged
		{Rect{-10, -10, 200, 200}, r},                // around: r unchanged
		{Rect{100, 0, 10, 50}, Rect{}},               // touching an edge
		{Rect{100, 50, 10, 10}, Rect{}},              // touching a corner
		{Rect{300, 300, 10, 10}, Rect{}},             // apart
		{Rect{10, 10, 0, 20}, Rect{}},                // empty, though inside
		{Rect{10, 10, -5, 20}, Rect{}},
	} {
		if got := r.Intersect(tc.o); got != tc.want {
			t.Errorf("Intersect(%v) = %v, want %v", tc.o, got, tc.want)
		}
		if got := tc.o.Intersect(r); got != tc.want {
			t.Errorf("Intersect(%v) the other way = %v, want %v", tc.o, got, tc.want)
		}
		if got, want := r.Overlaps(tc.o), !tc.want.Empty(); got != want || tc.o.Overlaps(r) != want {
			t.Errorf("Overlaps(%v) = %v, want %v", tc.o, got, want)
		}
	}
}

func TestRectUnion(t *testing.T) {
	r := Rect{0, 0, 100, 50}
	for _, tc := range []struct {
		a, b, want Rect
	}{
		{r, Rect{60, 30, 100, 100}, Rect{0, 0, 160, 130}},
		{r, Rect{-20, 70, 10, 10}, Rect{-20, 0, 120, 80}},
		{r, Rect{10, 10, 20, 20}, r},
		{r, Rect{}, r},                // the zero Rect adds nothing...
		{Rect{}, r, r},                // ...from either side
		{r, Rect{500, 500, 0, 10}, r}, // nor does an empty one far away
		{Rect{5, 5, 0, 0}, Rect{}, Rect{5, 5, 0, 0}},
	} {
		if got := tc.a.Union(tc.b); got != tc.want {
			t.Errorf("%v.Union(%v) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
	var bounds Rect
	for _, b := range []Rect{{10, 10, 5, 5}, {40, -3, 2, 2}, {0, 20, 1, 1}} {
		bounds = bounds.Union(b)
	}
	if bounds != (Rect{0, -3, 42, 24}) {
		t.Errorf("gathered bounds %v", bounds)
	}
	// -0.2+(0.30000000000000004 - -0.2) rounds to 0.3, short of the right
	// edge, so the width is nudged to reach it.
	a, b := Rect{-0.2, 0, 0.1, 1}, Rect{0.1, 0, 0.2, 1}
	if u := a.Union(b); !u.Contains(a) || !u.Contains(b) {
		t.Errorf("%v.Union(%v) = %v leaves one out", a, b, u)
	}
}

func TestRectContains(t *testing.T) {
	r := Rect{0, 0, 100, 50}
	for _, tc := range []struct {
		o    Rect
		want bool
	}{
		{r, true},
		{Rect{10, 10, 20, 20}, true},
		{Rect{0, 0, 100, 10}, true},   // sharing edges
		{Rect{90, 10, 20, 20}, false}, // overhanging
		{Rect{-1, 0, 10, 10}, false},
		{Rect{500, 500, 0, 0}, true}, // empty: inside everything
		{Rect{500, 500, -3, 2}, true},
	} {
		if got := r.Contains(tc.o); got != tc.want {
			t.Errorf("Contains(%v) = %v, want %v", tc.o, got, tc.want)
		}
	}
	if line := (Rect{0, 0, 0, 50}); !line.Contains(Rect{0, 0, 0, 10}) || line.Contains(Rect{0, 0, 1, 1}) {
		t.Error("an empty rect should contain only empty rects")
	}
	for _, e := range []Rect{{}, {0, 0, 5, 0}, {0, 0, -1, 5}, {0, 0, math.NaN(), 5}} {
		if !e.Empty() {
			t.Errorf("%v is not Empty", e)
		}
	}
	if (Rect{0, 0, 0.5, 0.5}).Empty() {
		t.Error("a small rect is Empty")
	}
}
