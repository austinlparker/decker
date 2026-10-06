package decker

import (
	"fmt"
	"testing"
)

func tableCtx(step int) Ctx {
	return Ctx{W: 240, H: 67, T: 10, Step: step, StepT: Settled, Theme: testTheme}
}

func tableRows(n, cols int) [][]string {
	rows := make([][]string, n)
	for i := range rows {
		for j := 0; j < cols; j++ {
			rows[i] = append(rows[i], fmt.Sprintf("row %d column %d", i, j))
		}
	}
	return rows
}

func TestTableFitsOneSize(t *testing.T) {
	c := tableCtx(0)
	r := c.Rect(0.05, 0.2, 0.9, 0.7)
	small := Table{Header: []string{"a", "b", "c"}, Rows: tableRows(3, 3)}.layout(c, r)
	big := Table{Header: []string{"a", "b", "c"}, Rows: tableRows(14, 3)}.layout(c, r)
	if big.size >= small.size {
		t.Errorf("14 rows set at %d, 3 rows at %d: a larger table should get smaller text", big.size, small.size)
	}
	if max := c.Size(tableMaxText); small.size > max {
		t.Errorf("size %d above the maximum %d", small.size, max)
	}
	if min := c.SmallText(c.Theme.Body); big.size < min {
		t.Errorf("size %d below the smallest readable %d", big.size, min)
	}
	for name, l := range map[string]*tableLayout{"small": small, "big": big} {
		if l.h > r.H {
			t.Errorf("%s table is %.1f tall in a %.1f rect", name, l.h, r.H)
		}
		if got := len(l.rowY); got != len(l.lines) {
			t.Errorf("%s: %d row offsets for %d rows of cells", name, got, len(l.lines))
		}
	}
}

func TestTableWrapsWithinCells(t *testing.T) {
	c := tableCtx(0)
	r := c.Rect(0.05, 0.1, 0.4, 0.8)
	tb := Table{Rows: [][]string{{"a fairly long cell that has to wrap more than once in a narrow column", "x"}}, Weights: []float64{1, 1}}
	l := tb.layout(c, r)
	if n := len(l.lines[0][0]); n < 2 {
		t.Errorf("long cell wrapped to %d lines", n)
	}
	for j, ls := range l.lines[0] {
		for _, s := range ls {
			if w := c.Theme.Body.Measure(s, l.size); w > l.colW[j]-2*l.padX+1e-9 {
				t.Errorf("column %d line %q is %.1f wide in %.1f", j, s, w, l.colW[j]-2*l.padX)
			}
		}
	}
}

func TestTableWeights(t *testing.T) {
	c := tableCtx(0)
	r := c.Rect(0, 0, 1, 1)
	l := Table{Rows: [][]string{{"a", "b", "c"}}, Weights: []float64{1, 3}}.layout(c, r)
	if len(l.colW) != 3 || l.colW[1] != 3*l.colW[0] || l.colW[2] != l.colW[0] {
		t.Errorf("column widths %v, want weights 1 3 1", l.colW)
	}
}

func TestTableRagged(t *testing.T) {
	c := tableCtx(5)
	tables := map[string]Table{
		"empty":         {},
		"header only":   {Header: []string{"a", "b"}},
		"no header":     {Rows: [][]string{{"a"}, {"a", "b", "c"}, nil, {}}},
		"short header":  {Header: []string{"a"}, Rows: [][]string{{"a", "b", "c"}}},
		"extra weights": {Rows: [][]string{{"a"}}, Weights: []float64{1, 2, 3}, Align: []Align{Right, Center, Left, Left}},
		"zero weights":  {Rows: [][]string{{"a", "b"}}, Weights: []float64{0, 0}},
		"newlines":      {Rows: [][]string{{"two\nlines", ""}}},
		"highlight past": {Rows: [][]string{{"a"}}, Highlight: 9, Walk: false,
			Reveal: TableRevealRows},
		"walk past":  {Rows: tableRows(2, 2), Walk: true, Reveal: TableRevealCols},
		"zebra rule": {Rows: tableRows(4, 2), Zebra: true, Rules: true, Highlight: 2},
	}
	for name, tb := range tables {
		for _, r := range []Rect{c.Rect(0.1, 0.1, 0.8, 0.8), {}, {10, 10, 0, 50}, {10, 10, 50, 0}, {0, 0, 5, 5}} {
			p := NewPixels(c.W, 2*c.H, c.Theme.Background)
			w, h := tb.Draw(c, p, r)
			if w < 0 || h < 0 || h > r.H+1e-9 {
				t.Errorf("%s in %v: drew %.1f x %.1f", name, r, w, h)
			}
		}
	}
	if w, h := (Table{}).Draw(c, NewPixels(10, 10, c.Theme.Background), c.Frame()); w != 0 || h != 0 {
		t.Errorf("empty table is %v x %v", w, h)
	}
}

// inkOutside counts pixels that differ from the background outside r.
func inkOutside(p *Pixels, bg RGB, r Rect) int {
	n := 0
	for y := 0; y < p.H; y++ {
		for x := 0; x < p.W; x++ {
			fx, fy := float64(x), float64(y)
			if fx >= r.X-1 && fx <= r.Right()+1 && fy >= r.Y-1 && fy <= r.Bottom()+1 {
				continue
			}
			if p.At(x, y) != bg {
				n++
			}
		}
	}
	return n
}

func TestTableStaysInRect(t *testing.T) {
	c := tableCtx(9)
	bg := c.Theme.Background
	long := "an extraordinarily long cell, far too long to fit on one line however it wraps, in a tiny rectangle"
	for name, tb := range map[string]Table{
		"overflowing rows": {Header: []string{"name", "value"}, Rows: tableRows(60, 2), Zebra: true, Rules: true, Highlight: 3},
		"clipped cells":    {Rows: [][]string{{long, long}, {long, "Supercalifragilisticexpialidocious"}}, Weights: []float64{1, 1}},
		"aligned":          {Header: []string{"a", "b", "c"}, Rows: tableRows(5, 3), Align: []Align{Left, Center, Right}, Walk: true},
	} {
		r := c.Rect(0.2, 0.3, 0.3, 0.4)
		p := NewPixels(c.W, 2*c.H, bg)
		_, h := tb.Draw(c, p, r)
		if h > r.H+1e-9 {
			t.Errorf("%s: height %.1f in a %.1f rect", name, h, r.H)
		}
		if n := inkOutside(p, bg, r); n != 0 {
			t.Errorf("%s: %d pixels drawn outside the rect", name, n)
		}
	}
}

func TestTableOverflowDropsRows(t *testing.T) {
	c := tableCtx(0)
	r := c.Rect(0.1, 0.1, 0.8, 0.3)
	l := Table{Header: []string{"h"}, Rows: tableRows(80, 1)}.layout(c, r)
	if l.size != c.SmallText(c.Theme.Body) {
		t.Errorf("size %d, want the smallest readable %d", l.size, c.SmallText(c.Theme.Body))
	}
	if len(l.lines) >= 81 || len(l.lines) < 2 {
		t.Errorf("%d rows kept of 81", len(l.lines))
	}
	if l.h > r.H {
		t.Errorf("kept rows are %.1f tall in a %.1f rect", l.h, r.H)
	}
}

// rowInk counts pixels in layout row row of the table drawn at r that differ
// from the background.
func rowInk(tb Table, c Ctx, row int) int {
	r := c.Rect(0.05, 0.1, 0.9, 0.8)
	l := tb.layout(c, r)
	bg := c.Theme.Background
	p := NewPixels(c.W, 2*c.H, bg)
	tb.Draw(c, p, r)
	n := 0
	for y := int(r.Y + l.rowY[row] + 1); y < int(r.Y+l.rowY[row]+l.rowH[row]-1); y++ {
		for x := int(r.X); x < int(r.Right()); x++ {
			if p.At(x, y) != bg {
				n++
			}
		}
	}
	return n
}

func TestTableRevealRows(t *testing.T) {
	tb := Table{Header: []string{"a", "b"}, Rows: tableRows(3, 2), FirstStep: 1, Reveal: TableRevealRows}
	for step := 0; step < 6; step++ {
		c := tableCtx(step)
		if got := rowInk(tb, c, 0) > 0; got != (step >= 1) {
			t.Errorf("step %d: header drawn = %v", step, got)
		}
		for i := 0; i < 3; i++ {
			if got := rowInk(tb, c, i+1) > 0; got != (step >= 1+i) {
				t.Errorf("step %d: row %d drawn = %v", step, i, got)
			}
		}
	}
}

func TestTableRevealAll(t *testing.T) {
	tb := Table{Rows: tableRows(2, 2), FirstStep: 2}
	for step := 0; step < 4; step++ {
		for i := 0; i < 2; i++ {
			if got := rowInk(tb, tableCtx(step), i) > 0; got != (step >= 2) {
				t.Errorf("step %d: row %d drawn = %v", step, i, got)
			}
		}
	}
}

func TestTableRevealCols(t *testing.T) {
	tb := Table{Header: []string{"a", "b", "c"}, Rows: tableRows(2, 3), Reveal: TableRevealCols}
	r := tableCtx(0).Rect(0.05, 0.1, 0.9, 0.8)
	for step := 0; step < 5; step++ {
		c := tableCtx(step)
		l := tb.layout(c, r)
		bg := c.Theme.Background
		p := NewPixels(c.W, 2*c.H, bg)
		tb.Draw(c, p, r)
		for j := 0; j < 3; j++ {
			n := 0
			y0 := int(r.Y+l.rowY[1]) + 2 // clear of the header rule
			for y := y0; y < int(r.Y+l.h); y++ {
				for x := int(r.X + l.colX[j]); x < int(r.X+l.colX[j]+l.colW[j]); x++ {
					if p.At(x, y) != bg {
						n++
					}
				}
			}
			if got := n > 0; got != (step >= j) {
				t.Errorf("step %d: column %d drawn = %v", step, j, got)
			}
		}
	}
}

func TestTableHighlight(t *testing.T) {
	tb := Table{Rows: tableRows(4, 1), Walk: true, FirstStep: 1}
	for _, tc := range []struct {
		step, to, from int
	}{{0, -1, -1}, {1, 0, -1}, {2, 1, 0}, {4, 3, 2}, {9, 3, 2}} {
		h := tb.highlight(tableCtx(tc.step), 4)
		if h.to != tc.to || h.from != tc.from {
			t.Errorf("step %d: highlight %d from %d, want %d from %d", tc.step, h.to, h.from, tc.to, tc.from)
		}
	}
	// Mid-move the highlight sits between its two rows, and settles on the
	// new one.
	c := Ctx{W: 240, H: 67, Step: 3, StepT: 0.1, Theme: testTheme}
	if e := tb.highlight(c, 4).e; e <= 0 || e >= 1 {
		t.Errorf("mid-move progress %v", e)
	}
	if e := tb.highlight(tableCtx(3), 4).e; e != 1 {
		t.Errorf("settled progress %v", e)
	}
	if h := (Table{Highlight: 2}).highlight(tableCtx(0), 3); h.to != 1 || h.from != -1 || h.e != 1 {
		t.Errorf("static highlight %+v", h)
	}
	if h := (Table{}).highlight(tableCtx(0), 3); h.to != -1 {
		t.Errorf("no highlight gave %+v", h)
	}
}

func TestTableDrawIsCached(t *testing.T) {
	c := tableCtx(0)
	r := c.Rect(0.05, 0.1, 0.9, 0.8)
	tb := Table{Header: []string{"a", "b"}, Rows: tableRows(5, 2)}
	if tb.layout(c, r) != tb.layout(c, r) {
		t.Error("the fit was recomputed for identical input")
	}
	other := Table{Header: []string{"a", "b"}, Rows: tableRows(6, 2)}
	if tb.layout(c, r) == other.layout(c, r) {
		t.Error("different tables shared a fit")
	}
}
