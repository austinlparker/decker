package decker

import (
	"math"
	"math/rand/v2"
	"reflect"
	"strings"
	"testing"
)

func FuzzLayoutPartitions(f *testing.F) {
	f.Add(uint16(100), uint16(60), int16(10), []byte{1, 2, 3})
	f.Add(uint16(0), uint16(0), int16(0), []byte{})
	f.Add(uint16(10), uint16(10), int16(100), []byte{0, 255})
	rng := rand.New(rand.NewPCG(5, 6))
	for range 64 {
		weights := make([]byte, rng.IntN(9))
		for i := range weights {
			weights[i] = byte(rng.Uint32())
		}
		f.Add(uint16(rng.IntN(512)), uint16(rng.IntN(512)), int16(rng.IntN(256)-64), weights)
	}
	f.Fuzz(func(t *testing.T, wb, hb uint16, gb int16, data []byte) {
		r := Rect{-17, 23, float64(wb), float64(hb)}
		gap := float64(gb)
		if len(data) > 16 {
			data = data[:16]
		}
		weights := make([]float64, len(data))
		for i, b := range data {
			weights[i] = float64(int8(b))
		}
		for _, horizontal := range []bool{false, true} {
			out, start, length := r.Rows(gap, weights...), r.Y, r.H
			if horizontal {
				out, start, length = r.Cols(gap, weights...), r.X, r.W
			}
			if len(out) != len(weights) {
				t.Fatal("partition count changed")
			}
			var sizeSum, weightSum float64
			for _, w := range weights {
				weightSum += max(w, 0)
			}
			for i, box := range out {
				at, size := box.Y, box.H
				if horizontal {
					at, size = box.X, box.W
				}
				if math.Abs(at-start) > 1e-7 || size < 0 || !finite(size) {
					t.Fatalf("partition %d invalid: %v", i, box)
				}
				if weights[i] <= 0 && size != 0 {
					t.Fatal("nonpositive weight received space")
				}
				start += size + gap
				sizeSum += size
			}
			if len(weights) > 0 && weightSum > 0 && math.Abs(sizeSum-max(length-gap*float64(len(weights)-1), 0)) > 1e-7 {
				t.Fatal("partition lost available space")
			}
		}
		for _, cut := range []float64{-1, 0, gap, r.W, r.H, 1e6} {
			top, bottom := r.CutTop(cut)
			left, right := r.CutLeft(cut)
			bt, rest := r.CutBottom(cut)
			rt, remain := r.CutRight(cut)
			if top.H+bottom.H != r.H || left.W+right.W != r.W || bt.H+rest.H != r.H || rt.W+remain.W != r.W ||
				top.Bottom() != bottom.Y || left.Right() != right.X || rest.Bottom() != bt.Y || remain.Right() != rt.X {
				t.Fatal("cuts did not conserve size and shared edges")
			}
		}
	})
}

func FuzzRectSetOps(f *testing.F) {
	f.Add(0.0, 0.0, 100.0, 50.0, 60.0, 30.0, 100.0, 100.0)
	f.Add(0.0, 0.0, 100.0, 50.0, 100.0, 0.0, 10.0, 50.0) // touching
	f.Add(0.0, 0.0, 0.0, 0.0, 5.0, 5.0, 1.0, 1.0)        // the zero Rect
	f.Add(-0.2, 0.0, 0.1, 1.0, 0.1, 0.0, 0.2, 1.0)       // edges that don't round-trip
	f.Add(1.0, 2.0, -3.0, 4.0, math.NaN(), 0.0, 1.0, math.Inf(1))
	rng := rand.New(rand.NewPCG(9, 10))
	coord := func() float64 {
		// Tenths, as fractions of a canvas give, so edges round.
		return float64(rng.IntN(4000)-2000) / 10
	}
	for range 128 {
		f.Add(coord(), coord(), coord(), coord(), coord(), coord(), coord(), coord())
	}
	f.Fuzz(func(t *testing.T, ax, ay, aw, ah, bx, by, bw, bh float64) {
		a, b := Rect{ax, ay, aw, ah}, Rect{bx, by, bw, bh}
		i, u := a.Intersect(b), a.Union(b)
		for _, v := range []float64{ax, ay, aw, ah, bx, by, bw, bh} {
			if !finite(v) || math.Abs(v) > 1e12 {
				return // bad input only has to come back without hanging
			}
		}
		edges := func(r Rect) [4]float64 { return [4]float64{r.X, r.Y, r.Right(), r.Bottom()} }

		if i != b.Intersect(a) || a.Overlaps(b) != b.Overlaps(a) {
			t.Fatal("intersection depends on the order")
		}
		if i.Empty() != !a.Overlaps(b) {
			t.Fatalf("Intersect = %v but Overlaps = %v", i, a.Overlaps(b))
		}
		if i.Empty() && i != (Rect{}) {
			t.Fatalf("no overlap gave %v, not the zero Rect", i)
		}
		if !a.Contains(i) || !b.Contains(i) {
			t.Fatalf("%v ∩ %v = %v reaches outside", a, b, i)
		}

		if !u.Contains(a) || !u.Contains(b) || !u.Contains(i) {
			t.Fatalf("%v ∪ %v = %v leaves one out", a, b, u)
		}
		if edges(u) != edges(b.Union(a)) && !a.Empty() && !b.Empty() {
			t.Fatal("union depends on the order")
		}
		if !a.Empty() && !b.Empty() {
			tol := 1e-12 * (math.Abs(u.X) + math.Abs(u.Y) + u.W + u.H + 1)
			if u.X != min(a.X, b.X) || u.Y != min(a.Y, b.Y) ||
				u.Right()-max(a.Right(), b.Right()) > tol || u.Bottom()-max(a.Bottom(), b.Bottom()) > tol {
				t.Fatalf("%v ∪ %v = %v is bigger than it needs to be", a, b, u)
			}
		}
		if a.Union(Rect{}) != a || !a.Empty() && (Rect{}).Union(a) != a {
			t.Fatal("the zero Rect added something to a union")
		}

		if !a.Contains(a) || !a.Contains(Rect{}) {
			t.Fatal("a rect does not contain itself or the empty rect")
		}
		// A rect inside another is what they share, and the other is what
		// they cover.
		if a.Contains(b) && b.Right() > b.X && b.Bottom() > b.Y {
			if edges(a.Intersect(b)) != edges(b) || edges(u) != edges(a) || !a.Overlaps(b) {
				t.Fatalf("%v contains %v but meets it in %v and covers it with %v", a, b, a.Intersect(b), u)
			}
		}
	})
}

func FuzzFitAllCache(f *testing.F) {
	for _, p := range [][2]string{{"", ""}, {"a", "b"}, {"a\x00", "b"}, {"界", "e\u0301"}} {
		f.Add(p[0], p[1])
	}
	f.Fuzz(func(t *testing.T, a, b string) {
		if len(a) > 64 || len(b) > 64 {
			return
		}
		font := testTheme.Body
		for _, parts := range [][]string{{a + "\x00" + b}, {a, b}, nil, {""}, {a + "\x00", b}, {a, "\x00" + b}} {
			wantSize, wantLines := font.fit(parts, 80, 60, 12, DefaultLeading)
			gotSize, gotLines := FitAll(font, parts, 80, 60, 12)
			if gotSize != wantSize || !reflect.DeepEqual(gotLines, wantLines) {
				t.Fatalf("FitAll(%q) = (%d,%q), uncached = (%d,%q)", parts, gotSize, gotLines, wantSize, wantLines)
			}
			if len(gotLines) > 0 {
				gotLines[0] = "caller mutation"
				_, again := FitAll(font, parts, 80, 60, 12)
				if !reflect.DeepEqual(again, wantLines) {
					t.Fatal("caller corrupted fit cache")
				}
			}
		}
	})
}

func FuzzLabelCacheKey(f *testing.F) {
	f.Add("a", "b")
	f.Add("", "")
	f.Fuzz(func(t *testing.T, a, b string) {
		if len(a)+len(b) > 256 {
			return
		}
		sets := [][]string{nil, {""}, {a, "\x00" + b}, {a + "\x00", b}, {a + "\x00" + b}}
		for i, x := range sets {
			for _, y := range sets[i+1:] {
				if !reflect.DeepEqual(x, y) && labelsKey(x) == labelsKey(y) {
					t.Fatalf("different labels %q and %q share a cache key", x, y)
				}
			}
		}
	})
}

func FuzzWrapConservesText(f *testing.F) {
	for _, s := range []string{"", "\n\n", "one two three four", "a\tb\n c", "界 e\u0301", "overlongword"} {
		f.Add(s, uint8(20))
	}
	f.Fuzz(func(t *testing.T, s string, width uint8) {
		if len(s) > 256 {
			return
		}
		measure := func(s string) float64 { return float64(len([]rune(s))) }
		maxW := float64(width)
		got := wrapBalanced(s, maxW, measure)
		if strings.Join(strings.Fields(strings.Join(got, " ")), " ") != strings.Join(strings.Fields(s), " ") {
			t.Fatalf("wrapping lost or reordered words: %q -> %q", s, got)
		}
		if len(got) != len(wrapGreedy(s, maxW, measure)) {
			t.Fatal("balancing changed greedy line count")
		}
		for _, line := range got {
			if measure(line) > maxW && len(strings.Fields(line)) > 1 {
				t.Fatalf("multi-word line %q exceeded width %g", line, maxW)
			}
		}
	})
}
