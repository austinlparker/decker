package decker

import (
	"math"
	"strconv"
	"strings"
	"testing"
)

func TestNiceScale(t *testing.T) {
	for _, tc := range []struct {
		lo, hi         float64
		maxTicks       int
		wantLo, wantHi float64
		wantStep       float64
	}{
		{0, 100, 5, 0, 100, 20},
		{0, 7, 5, 0, 8, 2},
		{0, 0.93, 5, 0, 1, 0.2},
		{-3, 5, 5, -4, 6, 2},
		{0, 1234, 6, 0, 1500, 500},
		{0, 0, 5, 0, 1, 0.2},    // no data: a unit axis
		{40, 40, 5, 20, 60, 10}, // a constant: centered, not collapsed
		{1e-3, 4e-3, 5, 1e-3, 4e-3, 1e-3},
	} {
		got := niceScale(tc.lo, tc.hi, tc.maxTicks)
		if math.Abs(got.lo-tc.wantLo) > 1e-9 || math.Abs(got.hi-tc.wantHi) > 1e-9 || math.Abs(got.step-tc.wantStep) > 1e-9 {
			t.Errorf("niceScale(%v, %v, %d) = %+v, want lo %v hi %v step %v", tc.lo, tc.hi, tc.maxTicks, got, tc.wantLo, tc.wantHi, tc.wantStep)
		}
		if got.lo > tc.lo+1e-9 || got.hi < tc.hi-1e-9 {
			t.Errorf("niceScale(%v, %v) = %+v does not cover the data", tc.lo, tc.hi, got)
		}
	}
}

func TestNiceScaleTicks(t *testing.T) {
	// Every step is 1, 2 or 5 times a power of ten, whatever the data.
	for _, hi := range []float64{0.37, 1, 3, 9.9, 47, 100, 640, 12345, 7.7e7} {
		sc := niceScale(0, hi, 5)
		ticks := sc.ticks()
		if len(ticks) < 2 || len(ticks) > 7 {
			t.Errorf("hi=%v: %d ticks %v", hi, len(ticks), ticks)
		}
		m := sc.step / math.Pow(10, math.Floor(math.Log10(sc.step)))
		if math.Abs(m-1) > 1e-9 && math.Abs(m-2) > 1e-9 && math.Abs(m-5) > 1e-9 {
			t.Errorf("hi=%v: step %v is not 1, 2 or 5 times a power of ten", hi, sc.step)
		}
		if ticks[0] != sc.lo || math.Abs(ticks[len(ticks)-1]-sc.hi) > 1e-9*sc.hi {
			t.Errorf("hi=%v: ticks %v do not span %v..%v", hi, ticks, sc.lo, sc.hi)
		}
	}
	// An explicit top stays where it is, with gridlines at round numbers below.
	sc := fixedScale(0, 90, 5)
	if sc.hi != 90 || sc.step != 20 {
		t.Errorf("fixedScale(0, 90) = %+v", sc)
	}
	if got := sc.ticks(); len(got) != 5 || got[4] != 80 {
		t.Errorf("fixedScale ticks = %v, want 0..80", got)
	}
}

func TestDefaultFormat(t *testing.T) {
	for v, want := range map[float64]string{
		0: "0", 3: "3", 0.30000000000000004: "0.3", 12.5: "12.5", 1500: "1500",
		12500: "12.5k", 2e6: "2M", -4e9: "-4B", 0.005: "0.005", -0.0000001: "0",
	} {
		if got := defaultFormat(v); got != want {
			t.Errorf("defaultFormat(%v) = %q, want %q", v, got, want)
		}
	}
}

func TestSeriesColor(t *testing.T) {
	th := *testTheme
	th.Series = nil
	want := []RGB{th.Accent, th.Accent2, th.Good, th.Warn, th.Muted}
	for i := -5; i < 12; i++ {
		if got := th.SeriesColor(i); got != want[((i%5)+5)%5] {
			t.Errorf("fallback SeriesColor(%d) = %v", i, got)
		}
	}
	th.Series = []RGB{Hex("#111111"), Hex("#222222"), Hex("#333333")}
	for i := -3; i < 8; i++ {
		if got := th.SeriesColor(i); got != th.Series[((i%3)+3)%3] {
			t.Errorf("SeriesColor(%d) = %v", i, got)
		}
	}
}

// chartCtx is a ctx at build step 0, since seconds into it.
func chartCtx(w, h int, since float64) Ctx {
	return Ctx{W: w, H: h, T: since, Step: 0, StepT: since, Theme: testTheme}
}

func noNaN(t *testing.T, p *Pixels, what string) {
	t.Helper()
	for i, px := range p.Pix {
		if math.IsNaN(float64(px.R)) || math.IsNaN(float64(px.G)) || math.IsNaN(float64(px.B)) {
			t.Fatalf("%s: NaN pixel at %d,%d", what, i%p.W, i/p.W)
		}
	}
}

// TestChartsSurviveBadData draws every chart with data that has nothing to
// scale to, or that breaks arithmetic, at several sizes and moments.
func TestChartsSurviveBadData(t *testing.T) {
	nan, inf := math.NaN(), math.Inf(1)
	datasets := map[string][]float64{
		"nil":      nil,
		"empty":    {},
		"zeros":    {0, 0, 0},
		"one":      {7},
		"negative": {-3, 4, -8, 0},
		"allneg":   {-1, -2, -3},
		"nan":      {nan, 2, nan, 5},
		"allnan":   {nan, nan},
		"inf":      {inf, 1, -inf},
		"huge":     {1e300, -1e300},
		"tiny":     {1e-300, 2e-300},
		"constant": {5, 5, 5, 5},
	}
	for name, data := range datasets {
		labels := []string{"alpha", "a rather long category label", "c", "", "e"}
		for _, sz := range [][2]int{{200, 60}, {40, 12}, {682, 171}, {3, 2}} {
			for _, since := range []float64{-1, 0, 0.1, 0.5, 2, Settled} {
				c := chartCtx(sz[0], sz[1], since)
				if since < 0 {
					c.Step, c.StepT = -1, 0 // before the charts' step
				}
				p := NewPixels(sz[0], 2*sz[1], c.Theme.Background)
				r := c.Frame().Inset(2, 2)
				what := name + " " + strconv.Itoa(sz[0]) + " " + strconv.FormatFloat(since, 'f', 1, 64)
				for _, horiz := range []bool{false, true} {
					BarChart{Labels: labels, Values: data, ShowValues: true, Horizontal: horiz}.Draw(c, p, r)
					BarChart{Labels: labels, Series: [][]float64{data, data}, Names: []string{"x"}, Max: 10, Horizontal: horiz}.Draw(c, p, r)
				}
				LineChart{Labels: labels, Series: [][]float64{data, data}, Points: true}.Draw(c, p, r)
				LineChart{Series: [][]float64{data}, Min: -1, Max: 1}.Draw(c, p, r)
				DonutChart{Labels: labels, Values: data, Center: "hello\nworld"}.Draw(c, p, r)
				DonutChart{Values: data, Thickness: 5}.Draw(c, p, r)
				Sparkline(c, p, r, data, c.Theme.Accent, Ease(since, 1))
				for _, v := range data {
					Stat{Value: v, Label: "stat", Decimals: 1, Prefix: "$", Suffix: "%"}.Draw(c, p, r)
				}
				noNaN(t, p, what)
			}
		}
	}
	// Nothing at all, and rects with no size or too much.
	c := chartCtx(100, 30, 1)
	p := NewPixels(100, 60, c.Theme.Background)
	for _, r := range []Rect{{}, {X: 5, Y: 5}, {X: -10, Y: -10, W: 500, H: 500}} {
		BarChart{}.Draw(c, p, r)
		LineChart{}.Draw(c, p, r)
		DonutChart{}.Draw(c, p, r)
		Sparkline(c, p, r, nil, c.Theme.Accent, 1)
		Stat{}.Draw(c, p, r)
	}
}

func countColor(p *Pixels, col RGB, y0, y1 int) (n int) {
	for y := y0; y < y1; y++ {
		for x := 0; x < p.W; x++ {
			if p.At(x, y) == col {
				n++
			}
		}
	}
	return n
}

func TestBarChartNegativeValuesHangBelowZero(t *testing.T) {
	c := chartCtx(200, 60, Settled)
	p := NewPixels(200, 120, c.Theme.Background)
	BarChart{Labels: []string{"up", "down"}, Values: []float64{10, -10}}.Draw(c, p, c.Frame())
	col := c.Theme.SeriesColor(0)
	// The bars are the same size, so the axis is centered on zero: one bar
	// above the middle of the plot and one below.
	top, bottom := countColor(p, col, 0, 55), countColor(p, col, 65, 120)
	if top < 100 || bottom < 100 {
		t.Errorf("solid bar pixels above and below zero = %d, %d; want both bars drawn", top, bottom)
	}
}

func TestChartWaitsForItsStep(t *testing.T) {
	c := chartCtx(100, 30, 1)
	p := NewPixels(100, 60, c.Theme.Background)
	blank := append([]RGB(nil), p.Pix...)
	for name, draw := range map[string]func(Ctx, *Pixels, Rect) (float64, float64){
		"bar":   BarChart{Values: []float64{1, 2}, Step: 2}.Draw,
		"line":  LineChart{Series: [][]float64{{1, 2}}, Step: 2}.Draw,
		"donut": DonutChart{Values: []float64{1, 2}, Step: 2}.Draw,
		"stat":  Stat{Value: 5, Step: 2}.Draw,
	} {
		if w, h := draw(c, p, c.Frame()); w != 0 || h != 0 {
			t.Errorf("%s drew a %vx%v size before its step", name, w, h)
		}
	}
	for i := range blank {
		if p.Pix[i] != blank[i] {
			t.Fatal("a chart drew before its step")
		}
	}
	c.Step = 2
	if w, h := (BarChart{Values: []float64{1, 2}, Step: 2}).Draw(c, p, c.Frame()); w == 0 || h == 0 {
		t.Error("a chart drew nothing at its step")
	}
}

// TestBarsGrowFromTheBaseline checks the bars start empty and only get taller.
func TestBarsGrowFromTheBaseline(t *testing.T) {
	prev := -1
	for _, since := range []float64{0, 0.3, 0.5, 0.8, 1.2, 3} {
		c := chartCtx(200, 60, since)
		p := NewPixels(200, 120, c.Theme.Background)
		BarChart{Labels: []string{"a", "b", "c"}, Values: []float64{3, 5, 4}}.Draw(c, p, c.Frame())
		n := countColor(p, c.Theme.SeriesColor(0), 0, 120)
		if n < prev || (since == 0 && n != 0) {
			t.Errorf("since %v: %d bar pixels after %d", since, n, prev)
		}
		prev = n
	}
	if prev == 0 {
		t.Error("the bars never grew")
	}
}

func TestStatCountsUp(t *testing.T) {
	number := func(s Stat, since float64) float64 {
		txt := strings.TrimSuffix(strings.TrimPrefix(s.text(since), s.Prefix), s.Suffix)
		v, err := strconv.ParseFloat(txt, 64)
		if err != nil {
			t.Fatalf("%q: %v", txt, err)
		}
		return v
	}
	for _, s := range []Stat{
		{Value: 1234567},
		{Value: 99.5, Decimals: 1, Prefix: "$", Suffix: "M"},
		{Value: 0.037, Decimals: 3, Duration: 0.4},
		{Value: -250, Decimals: 2, Suffix: "%"},
		{Value: 0},
	} {
		if got := number(s, 0); got != 0 {
			t.Errorf("%+v starts at %v, want 0", s, got)
		}
		sign := 1.0
		if s.Value < 0 {
			sign = -1
		}
		prev := 0.0
		for since := 0.0; since <= 3; since += 1.0 / 60 {
			v := number(s, since) * sign
			if v < prev {
				t.Fatalf("%+v: %v after %v at %.3fs: not monotonic", s, v, prev, since)
			}
			prev = v
		}
		want := strconv.FormatFloat(s.Value, 'f', s.Decimals, 64)
		if got := s.text(Settled); got != s.Prefix+want+s.Suffix {
			t.Errorf("%+v ends at %q, want %q", s, got, s.Prefix+want+s.Suffix)
		}
		if got := s.text(s.duration()); got != s.text(Settled) {
			t.Errorf("%+v is still counting at its duration: %q", s, got)
		}
	}
}

// TestStatKeepsItsWidth checks digits take equal cells, so a number of a given
// length occupies the same width whatever it says, and the final value is the
// widest.
func TestStatKeepsItsWidth(t *testing.T) {
	f := testTheme.Display
	_, w1 := tabular(f, 60, "1111")
	_, w2 := tabular(f, 60, "8080")
	_, w3 := tabular(f, 60, "0000")
	if w1 != w2 || w2 != w3 {
		t.Errorf("tabular widths differ: %v %v %v", w1, w2, w3)
	}
	s := Stat{Value: 98765.4, Decimals: 1, Prefix: "$"}
	_, final := tabular(f, 60, s.text(Settled))
	for since := 0.0; since < 2; since += 0.05 {
		if _, w := tabular(f, 60, s.text(since)); w > final+1e-9 {
			t.Fatalf("at %.2fs the number is %v wide, wider than the final %v", since, w, final)
		}
	}
}

func TestSparklineDrawsOnAndStaysInside(t *testing.T) {
	c := chartCtx(100, 30, 1)
	r := NewRect(10, 10, 60, 20)
	p := NewPixels(100, 60, c.Theme.Background)
	blank := append([]RGB(nil), p.Pix...)
	Sparkline(c, p, r, []float64{1, 3, 2, 5, 4}, c.Theme.Accent, 0)
	for i := range blank {
		if p.Pix[i] != blank[i] {
			t.Fatal("a sparkline at prog 0 drew something")
		}
	}
	Sparkline(c, p, r, []float64{1, 3, 2, 5, 4}, c.Theme.Accent, 1)
	drew := false
	for y := 0; y < p.H; y++ {
		for x := 0; x < p.W; x++ {
			if p.At(x, y) == blank[y*p.W+x] {
				continue
			}
			drew = true
			if float64(x) < r.X-1 || float64(x) > r.Right()+1 || float64(y) < r.Y-1 || float64(y) > r.Bottom()+1 {
				t.Fatalf("sparkline drew outside its rect at %d,%d", x, y)
			}
		}
	}
	if !drew {
		t.Error("a sparkline at prog 1 drew nothing")
	}
}
