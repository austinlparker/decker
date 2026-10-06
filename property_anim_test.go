package decker

import (
	"math"
	"strings"
	"testing"
)

func FuzzAnimationBounds(f *testing.F) {
	for _, p := range []float64{-math.MaxFloat64, -1, 0, math.SmallestNonzeroFloat64, 0.25, 0.5, 1, 10, 1e20, math.MaxFloat64} {
		f.Add(p)
	}
	f.Fuzz(func(t *testing.T, p float64) {
		if !finite(p) {
			return // NaN has no ordered progress; infinite clocks are undefined.
		}
		for _, ease := range []func(float64) float64{EaseInQuad, EaseOutQuad, EaseInOutQuad, EaseInCubic, EaseOutCubic, EaseInOutCubic, EaseOutExpo, EaseInOutExpo, EaseOutBounce} {
			v := ease(p)
			if !finite(v) || v < 0 || v > 1 || p <= 0 && v != 0 || p >= 1 && v != 1 {
				t.Fatalf("easing(%g) = %g outside clamped endpoints", p, v)
			}
		}
		if v := Progress(p, 0, 1); v != Clamp01(p) {
			t.Fatal("unit progress disagrees with clamp")
		}
		if v := Progress(p, 0, 0); (p < 0 && v != 0) || (p >= 0 && v != 1) {
			t.Fatal("instant progress has wrong endpoint")
		}
		if p >= 10 && Spring(2, 8, p, 4, 0.6) != 8 {
			t.Fatalf("Spring at settled time %g returned its start", p)
		}
		if p <= 0 && Spring(2, 8, p, 4, 0.6) != 2 {
			t.Fatal("spring moved before its start")
		}
	})
}

func FuzzAxisMapping(f *testing.F) {
	for _, v := range [][3]float64{{0, 1, 0.5}, {-3, 7, 0}, {-math.MaxFloat64, math.MaxFloat64, 0}, {-math.MaxFloat64, math.MaxFloat64, math.MaxFloat64}, {0, math.SmallestNonzeroFloat64, math.SmallestNonzeroFloat64}} {
		f.Add(v[0], v[1], v[2])
	}
	f.Fuzz(func(t *testing.T, lo, hi, v float64) {
		if !finite(lo) || !finite(hi) || !finite(v) || hi <= lo {
			return
		}
		a := axisScale{lo: lo, hi: hi}
		p := a.at(v)
		if !finite(p) || p < 0 || p > 1 || v <= lo && p != 0 || v >= hi && p != 1 {
			t.Fatalf("axis %+v maps %g to %g", a, v, p)
		}
		if a.at(lo) != 0 || a.at(hi) != 1 {
			t.Fatal("axis failed to preserve endpoints")
		}
		// For a symmetric axis, zero is exactly the midpoint even when the
		// subtraction of its endpoints would overflow.
		if lo == -hi && a.at(0) != 0.5 {
			t.Fatal("symmetric axis lost its midpoint")
		}
	})
}

func FuzzChartScale(f *testing.F) {
	for _, bounds := range [][2]float64{{0, 0}, {-3, 7}, {40, 40}, {-math.MaxFloat64, math.MaxFloat64}, {math.MaxFloat64, math.MaxFloat64}, {0, math.SmallestNonzeroFloat64}, {1e-300, 2e-300}, {1e300, 2e300}} {
		f.Add(bounds[0], bounds[1], uint8(5))
	}
	f.Fuzz(func(t *testing.T, x, y float64, nb uint8) {
		if !finite(x) || !finite(y) {
			return
		}
		lo, hi := min(x, y), max(x, y)
		for _, a := range []axisScale{niceScale(lo, hi, int(nb%8)+2), fixedScale(lo, hi, int(nb%8)+2)} {
			if !finite(a.lo) || !finite(a.hi) || !finite(a.step) || a.step <= 0 || a.hi <= a.lo {
				t.Fatalf("invalid scale for [%g,%g]: %+v", lo, hi, a)
			}
			if a.lo > lo || a.hi < hi {
				// The rounding tolerance on ordinary decimal endpoints can
				// lose a few ulps, but must never drop a measurable data range.
				eps := max(math.Abs(lo), math.Abs(hi)) * 1e-8
				if a.lo-lo > eps || hi-a.hi > eps {
					t.Fatalf("scale %+v does not cover [%g,%g]", a, lo, hi)
				}
			}
			ticks := a.ticks()
			if len(ticks) > 64 {
				t.Fatal("unbounded tick list")
			}
			for i, tick := range ticks {
				if !finite(tick) || i > 0 && tick <= ticks[i-1] {
					t.Fatalf("invalid ticks for %+v: %v", a, ticks)
				}
			}
			for _, v := range []float64{lo, hi} {
				p := a.at(v)
				if !finite(p) || p < 0 || p > 1 {
					t.Fatal("scale cannot map its input")
				}
			}
		}
		for _, v := range []float64{lo, hi} {
			if text := defaultFormat(v); strings.Contains(text, "Inf") || strings.Contains(text, "NaN") {
				t.Fatalf("finite value %g formatted as %q", v, text)
			}
		}
	})
}
