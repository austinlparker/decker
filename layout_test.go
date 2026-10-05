package decker

import "testing"

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
	if got := r.Place(20, 10, 1, 0.5); got != (Rect{80, 20, 20, 10}) {
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
