package decker

import (
	"math"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
)

func ticksOf(s Scale) []float64 { return slices.Collect(s.Ticks()) }

func TestNiceScaleMatchesCharts(t *testing.T) {
	for _, tc := range []struct {
		lo, hi           float64
		maxTicks         int
		wantMin, wantMax float64
		wantStep         float64
		wantTicks        int
		wantLast         float64
	}{
		{0, 100, 5, 0, 100, 20, 6, 100},
		{0, 7, 5, 0, 8, 2, 5, 8},
		{-3, 5, 5, -4, 6, 2, 6, 6},
		{0, 1234, 6, 0, 1500, 500, 4, 1500},
		{0, 0, 5, 0, 1, 0.2, 6, 1},     // no data: a unit axis
		{40, 40, 5, 20, 60, 10, 5, 60}, // a constant: centered, not collapsed
		{-8, -1, 5, -8, 0, 2, 5, 0},
	} {
		s := NiceScale(tc.lo, tc.hi, tc.maxTicks, 10, 110)
		if math.Abs(s.Min-tc.wantMin) > 1e-9 || math.Abs(s.Max-tc.wantMax) > 1e-9 || math.Abs(s.Step-tc.wantStep) > 1e-9 {
			t.Errorf("NiceScale(%v, %v, %d) = %+v, want %v..%v step %v", tc.lo, tc.hi, tc.maxTicks, s, tc.wantMin, tc.wantMax, tc.wantStep)
		}
		if s.From != 10 || s.To != 110 {
			t.Errorf("NiceScale(%v, %v) lost its pixels: %+v", tc.lo, tc.hi, s)
		}
		ticks := ticksOf(s)
		if len(ticks) != tc.wantTicks || math.Abs(ticks[len(ticks)-1]-tc.wantLast) > 1e-9 {
			t.Errorf("NiceScale(%v, %v) ticks %v, want %v ending at %v", tc.lo, tc.hi, ticks, tc.wantTicks, tc.wantLast)
		}
		if a := niceScale(min(tc.lo, tc.hi), max(tc.lo, tc.hi), tc.maxTicks); !slices.Equal(ticks, a.ticks()) {
			t.Errorf("NiceScale(%v, %v) ticks %v, a chart's %v", tc.lo, tc.hi, ticks, a.ticks())
		}
	}
}

func TestNiceScaleTakesBadData(t *testing.T) {
	nan, inf := math.NaN(), math.Inf(1)
	unit := NiceScale(0, 0, 5, 0, 1)
	for _, tc := range []struct {
		lo, hi float64
		want   Scale
	}{
		{nan, nan, unit},
		{-inf, inf, unit},
		{nan, 40, NiceScale(40, 40, 5, 0, 1)},
		{40, inf, NiceScale(40, 40, 5, 0, 1)},
		{7, 0, NiceScale(0, 7, 5, 0, 1)}, // either order
	} {
		if got := NiceScale(tc.lo, tc.hi, 5, 0, 1); got != tc.want {
			t.Errorf("NiceScale(%v, %v) = %+v, want %+v", tc.lo, tc.hi, got, tc.want)
		}
	}
	for _, r := range [][2]float64{{1e300, -1e300}, {-math.MaxFloat64, math.MaxFloat64}, {1e-300, 2e-300}, {0, math.SmallestNonzeroFloat64}, {math.MaxFloat64, math.MaxFloat64}} {
		s := NiceScale(r[0], r[1], 6, 0, 100)
		if !finite(s.Min) || !finite(s.Max) || !(s.Max > s.Min) || !(s.Step > 0) {
			t.Errorf("NiceScale(%v, %v) = %+v", r[0], r[1], s)
		}
		ticks := ticksOf(s)
		if len(ticks) < 2 {
			t.Errorf("NiceScale(%v, %v) has ticks %v", r[0], r[1], ticks)
		}
		for _, v := range ticks {
			if x := s.At(v); !finite(x) || x < 0 || x > 100 {
				t.Errorf("NiceScale(%v, %v) puts tick %v at %v", r[0], r[1], v, x)
			}
			if l := s.Label(v); l == "" || len(l) > 24 || strings.Contains(l, "Inf") || strings.Contains(l, "NaN") {
				t.Errorf("NiceScale(%v, %v) labels %v as %q", r[0], r[1], v, l)
			}
		}
	}
}

func TestScaleAt(t *testing.T) {
	x := Scale{Min: 0, Max: 200, Step: 50, From: 100, To: 500}
	for v, want := range map[float64]float64{0: 100, 200: 500, 50: 200, -50: 0, 300: 700} {
		if got := x.At(v); got != want {
			t.Errorf("At(%v) = %v, want %v", v, got, want)
		}
	}
	// A y axis runs upward: To above From.
	y := Scale{Min: -1, Max: 1, From: 80, To: 20}
	if got := y.At(0); got != 50 {
		t.Errorf("upward At(0) = %v, want 50", got)
	}
	if got := y.At(2); got != -10 {
		t.Errorf("upward At(2) = %v, want -10", got)
	}
	if got := x.At(math.NaN()); !math.IsNaN(got) {
		t.Errorf("At(NaN) = %v, want NaN", got)
	}
	if got := x.At(math.Inf(1)); !math.IsInf(got, 1) {
		t.Errorf("At(+Inf) = %v, want +Inf", got)
	}
	for _, s := range []Scale{{}, {Min: 5, Max: 5, From: 3, To: 9}, {Min: 5, Max: 1, From: 3, To: 9}, {Min: math.NaN(), Max: 1, From: 3}} {
		if got := s.At(4); got != s.From {
			t.Errorf("%+v.At(4) = %v, want From", s, got)
		}
	}
	// Ends wider apart than MaxFloat64 still map finitely, zero in the middle.
	wide := Scale{Min: -math.MaxFloat64, Max: math.MaxFloat64, From: 0, To: 100}
	if got := wide.At(0); got != 50 {
		t.Errorf("wide At(0) = %v, want 50", got)
	}
}

func TestScaleTicks(t *testing.T) {
	for _, tc := range []struct {
		s    Scale
		want []float64
	}{
		{Scale{Min: 0, Max: 347, Step: 100}, []float64{0, 100, 200, 300}}, // the data's own ends, a round step
		{Scale{Min: -0.3, Max: 0.3, Step: 0.2}, []float64{-0.2, 0, 0.2}},
		{Scale{Min: 0.6, Max: 0.9, Step: 0.1}, []float64{0.6, 0.7000000000000001, 0.8, 0.9}}, // 6*0.1 and 9*0.1 put back on the ends

		{Scale{Min: 0, Max: 1}, nil},                     // no step, no ticks
		{Scale{Min: 1, Max: 0, Step: 0.5}, nil},          // inverted
		{Scale{Min: 0, Max: 1, Step: math.NaN()}, nil},   // broken step
		{Scale{Min: math.Inf(-1), Max: 1, Step: 1}, nil}, // infinite end
		{Scale{Min: 3, Max: 3, Step: 1}, []float64{3}},
		{Scale{Min: 1e300, Max: 1e300 * 1.0000001, Step: 1}, []float64{1e300, 1e300 * 1.0000001}}, // too fine to count
	} {
		if got := ticksOf(tc.s); !slices.Equal(got, tc.want) {
			t.Errorf("%+v ticks %v, want %v", tc.s, got, tc.want)
		}
	}
	if n := len(ticksOf(Scale{Min: 0, Max: 1e9, Step: 1})); n != scaleTicks {
		t.Errorf("a step far too fine gave %d ticks, want the cap of %d", n, scaleTicks)
	}
	// Asking NiceScale for too many ticks still reaches the far end.
	for _, hi := range []float64{1, 7, 1234} {
		s := NiceScale(0, hi, 1e9, 0, 1)
		if ticks := ticksOf(s); len(ticks) > scaleTicks || ticks[len(ticks)-1] != s.Max {
			t.Errorf("NiceScale(0, %v, 1e9) = %+v has %d ticks ending at %v", hi, s, len(ticks), ticks[len(ticks)-1])
		}
	}
	// Stopping early stops the ticks.
	n := 0
	for range NiceScale(0, 100, 10, 0, 1).Ticks() {
		n++
		if n == 2 {
			break
		}
	}
	if n != 2 {
		t.Errorf("break after 2 ticks counted %d", n)
	}
	if got := ticksOf(Scale{Min: -2, Max: -0.0, Step: 1}); got[len(got)-1] != 0 || math.Signbit(got[len(got)-1]) {
		t.Errorf("ticks %v end in -0", got)
	}
}

func TestScaleLabel(t *testing.T) {
	for _, tc := range []struct {
		step, v float64
		want    string
	}{
		{0.25, 0.5, "0.50"},
		{0.25, 1, "1.00"},
		{50, 150, "150"},
		{0.1, 0.1 * 3, "0.3"},
		{0.2, 3 * 0.2, "0.6"},
		{5e-5, 1e-4, "0.00010"},
		{1, 7, "7"},
		{1, 0.6, "1"},        // between ticks, at the step's precision
		{0.1, -0.04, "0.0"},  // rounds to zero: no "-0"
		{0.1, -1e-17, "0.0"}, // float noise around zero
		{2, -4, "-4"},        // negative values keep their sign
		{0.01, -0.03, "-0.03"},
		{2e-7, 4e-7, "4e-07"},       // finer than six decimals
		{5e14, 1.5e15, "1.5e+15"},   // past 15 digits
		{2.5e20, 7.5e20, "7.5e+20"}, // the mantissa reaches the step's last digit
		{2.5e20, 0, "0"},
		{1e300, 3e300, "3e+300"},
		{1e308, 1e308, "1e+308"},
		{0.1, math.NaN(), ""},
		{0.1, math.Inf(-1), ""},
		{0.1, 0, "0.0"},
		{0, 1.0 / 3, "0.333333333333333"}, // no step: 15 digits
		{math.NaN(), 12500, "12500"},
		{0, 1e20, "1e+20"},
		{0, math.Copysign(0, -1), "0"},
		{0.30000000000000004, 0.9, "0.9"},       // a step with float noise
		{5.000000000000001e-7, 1e-6, "1.0e-06"}, // the same, finer than six decimals
	} {
		s := Scale{Min: 0, Max: max(1, math.Abs(tc.v)), Step: tc.step}
		if !finite(s.Max) {
			s.Max = 1
		}
		if got := s.Label(tc.v); got != tc.want {
			t.Errorf("step %v: Label(%v) = %q, want %q", tc.step, tc.v, got, tc.want)
		}
	}
	// One axis writes every tick with the same decimals.
	s := NiceScale(0, 1, 5, 0, 1)
	var got []string
	for v := range s.Ticks() {
		got = append(got, s.Label(v))
	}
	if want := []string{"0.0", "0.2", "0.4", "0.6", "0.8", "1.0"}; !slices.Equal(got, want) {
		t.Errorf("labels %q, want %q", got, want)
	}
}

func TestScaleAllocatesNothing(t *testing.T) {
	s := NiceScale(-3, 47, 6, 0, 640)
	sum := 0.0
	if n := testing.AllocsPerRun(50, func() {
		for v := range s.Ticks() {
			sum += s.At(v)
		}
	}); n != 0 {
		t.Errorf("ranging over Ticks and placing them allocated %v times", n)
	}
	_ = sum
}

func FuzzScale(f *testing.F) {
	for _, b := range [][2]float64{{0, 0}, {-3, 7}, {40, 40}, {7, -3}, {-math.MaxFloat64, math.MaxFloat64}, {math.MaxFloat64, math.MaxFloat64}, {0, math.SmallestNonzeroFloat64}, {1e-300, 2e-300}, {1e300, 2e300}, {math.NaN(), 3}, {math.Inf(-1), math.Inf(1)}} {
		f.Add(b[0], b[1], uint8(5), 0.0, 640.0)
	}
	rng := rand.New(rand.NewPCG(7, 8))
	for range 64 {
		e := float64(rng.IntN(40) - 20)
		lo, hi := (rng.Float64()*2-1)*math.Pow(10, e), (rng.Float64()*2-1)*math.Pow(10, e)
		f.Add(lo, hi, uint8(rng.IntN(12)), rng.Float64()*500, rng.Float64()*500)
	}
	f.Fuzz(func(t *testing.T, lo, hi float64, nb uint8, from, to float64) {
		if !finite(from) || !finite(to) || math.Abs(from) > 1e9 || math.Abs(to) > 1e9 {
			return // pixel positions are on or near a canvas
		}
		maxTicks := int(nb%12) + 2
		s := NiceScale(lo, hi, maxTicks, from, to)
		if !finite(s.Min) || !finite(s.Max) || !finite(s.Step) || !(s.Step > 0) || !(s.Max > s.Min) {
			t.Fatalf("NiceScale(%g, %g, %d) = %+v", lo, hi, maxTicks, s)
		}
		// Finite data lies inside the scale, give or take the charts' rounding
		// tolerance (see FuzzChartScale).
		eps := 0.0
		for _, v := range []float64{lo, hi} {
			if finite(v) {
				eps = max(eps, math.Abs(v)*1e-8)
			}
		}
		for _, v := range []float64{lo, hi} {
			if finite(v) && (s.Min-v > eps || v-s.Max > eps) {
				t.Fatalf("NiceScale(%g, %g) = %+v leaves out %g", lo, hi, s, v)
			}
		}
		if s.At(s.Min) != from || s.At(s.Max) != to {
			t.Fatalf("%+v does not map its ends onto its pixels", s)
		}
		// The ticks are a chart's gridlines, kept on the ends, increasing, and
		// placed in order between the ends.
		ticks := ticksOf(s)
		if finite(lo) && finite(hi) {
			want := niceScale(min(lo, hi), max(lo, hi), maxTicks).ticks()
			if len(ticks) != len(want) {
				t.Fatalf("%+v ticks %v, a chart's %v", s, ticks, want)
			}
			for i, w := range want {
				if math.Abs(ticks[i]-w) > max(s.Step*1e-9, math.Abs(w)*1e-15) {
					t.Fatalf("%+v ticks %v, a chart's %v", s, ticks, want)
				}
			}
		}
		lower, upper := min(from, to), max(from, to)
		prevV, prevX := math.Inf(-1), 0.0
		for i, v := range ticks {
			x := s.At(v)
			if !finite(v) || v <= prevV || v < s.Min || v > s.Max || x < lower || x > upper {
				t.Fatalf("%+v tick %d of %v is %g at %g", s, i, ticks, v, x)
			}
			if i > 0 && (to > from && x < prevX || to < from && x > prevX) {
				t.Fatalf("%+v places tick %g at %g, before the one before", s, v, x)
			}
			if l := s.Label(v); l == "" || len(l) > 24 {
				t.Fatalf("%+v labels %g as %q", s, v, l)
			}
			prevV, prevX = v, x
		}
		// At is monotonic and extrapolates past the ends.
		vs := []float64{lo, hi, s.Min, s.Max, s.Min - s.Step, s.Max + s.Step, (s.Min + s.Max) / 2}
		for _, a := range vs {
			for _, b := range vs {
				if !finite(a) || !finite(b) || a >= b {
					continue
				}
				xa, xb := s.At(a), s.At(b)
				if to > from && xa > xb || to < from && xa < xb {
					t.Fatalf("%+v places %g at %g but %g at %g", s, a, xa, b, xb)
				}
			}
		}
	})
}
