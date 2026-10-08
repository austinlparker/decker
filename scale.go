package decker

import (
	"iter"
	"math"
	"strconv"
	"strings"
)

// Scale maps data values onto a span of pixels: the axis of a visual drawn by
// hand, like a trace waterfall's milliseconds. It draws nothing. At places a
// value, Ticks lists round values to mark and Label writes them:
//
//	ms := NiceScale(0, trace.Dur, 6, plot.X, plot.Right())
//	for v := range ms.Ticks() {
//		x := ms.At(v)
//		p.Rect(x, plot.Bottom(), 1, c.Unit(0.01), c.Theme.Faint, 1)
//		Text{Font: c.Theme.Body, Size: c.SmallText(c.Theme.Body), Color: c.Theme.Muted, Align: Center}.
//			Draw(p, ms.Label(v)+"ms", x, plot.Bottom()+c.Unit(0.015))
//	}
//
// NiceScale picks ticks the way the charts' value axes do, so a hand-drawn
// axis agrees with a BarChart beside it. A Scale is a plain value: build one by
// hand, or adjust what NiceScale returns.
type Scale struct {
	Min, Max float64 // the data range shown; NiceScale rounds it out to whole steps
	Step     float64 // the distance between ticks; 0 for none
	From, To float64 // the pixel positions of Min and Max; To < From runs upward, for a y axis
}

// NiceScale returns a Scale covering lo..hi with at most about maxTicks ticks
// (up to 500) at round numbers (1, 2 or 5 times a power of ten), its ends
// moved out to whole steps, mapped onto the pixels from..to.
//
// It takes data as the charts do: lo and hi may come in either order, a NaN or
// infinite end counts as missing, no data at all gives 0..1, and a constant is
// centered in a range around it, so the result always has Max > Min and a
// positive Step. To keep the data's own ends, say a trace that ran 347ms, set
// Min and Max back afterwards: the step and the ticks stay round.
func NiceScale(lo, hi float64, maxTicks int, from, to float64) Scale {
	switch {
	case !finite(lo) && !finite(hi):
		lo, hi = 0, 0
	case !finite(lo):
		lo = hi
	case !finite(hi):
		hi = lo
	}
	// Rounding the step can give half as many ticks again as asked for, and
	// 500 of those still fit under the cap on Ticks.
	a := niceScale(min(lo, hi), max(lo, hi), min(maxTicks, scaleTicks/2))
	return Scale{Min: a.lo, Max: a.hi, Step: a.step, From: from, To: to}
}

// At returns the pixel position of v: From at Min, To at Max, in proportion
// between them and beyond them. NaN gives NaN, so a missing value stays
// missing. A scale with no range (Max not above Min) puts every value at From.
func (s Scale) At(v float64) float64 {
	switch {
	case math.IsNaN(v):
		return v
	case !(s.Max > s.Min), v == s.Min:
		return s.From
	case v == s.Max:
		return s.To
	}
	u := (v - s.Min) / (s.Max - s.Min)
	if math.IsInf(s.Max-s.Min, 0) {
		// Finite ends can span more than MaxFloat64. Halving before
		// subtracting keeps the span and the position finite.
		u = (v/2 - s.Min/2) / (s.Max/2 - s.Min/2)
	}
	x := s.From + (s.To-s.From)*u
	// Rounding can carry x an ulp across From or To. Keeping it on the side
	// its value belongs keeps At monotonic: a larger value is never placed
	// before a smaller one.
	lo, hi := min(s.From, s.To), max(s.From, s.To)
	switch {
	case v < s.Min && s.To >= s.From, v > s.Max && s.To < s.From:
		return min(x, lo)
	case v < s.Min, v > s.Max:
		return max(x, hi)
	}
	return min(max(x, lo), hi)
}

// scaleTicks caps Ticks, so a Step far too small for its range can't stall a
// frame.
const scaleTicks = 1000

// Ticks yields the tick values in increasing order: the multiples of Step from
// Min to Max, the ends included when they fall on a step, at most 1000 of
// them. It yields nothing for a scale without a Step or with Max below Min,
// and only Min and Max when the step is too fine for the values' precision.
// Ranging over it allocates nothing.
func (s Scale) Ticks() iter.Seq[float64] {
	// A literal the compiler inlines into the range loop, unlike the method
	// value s.ticks, lets the loop body stay on the stack.
	return func(yield func(float64) bool) { s.ticks(yield) }
}

func (s Scale) ticks(yield func(float64) bool) {
	if !(s.Step > 0) || !finite(s.Step) || !finite(s.Min) || !finite(s.Max) || !(s.Max >= s.Min) {
		return
	}
	// The tolerances, the fallback and the -0 fix are the charts' (see
	// axisScale.ticks), so a scale's ticks match the gridlines of a chart's.
	// An end within that tolerance of a multiple counts as one, but k*Step
	// lands a few ulps off it (9*0.1 is past 0.9, 6*0.1 short of 0.6), so the
	// tick is put back on the end, where At places it exactly at From or To.
	k0 := math.Ceil(s.Min/s.Step - 1e-9)
	last := min(math.Floor(s.Max/s.Step+1e-9), k0+scaleTicks-1)
	// k and k*Step grow in size toward the ends of the run, so checking the
	// ends checks every tick.
	if !finite(k0*s.Step) || !finite(last*s.Step) || k0+1 == k0 || last+1 == last {
		if yield(s.Min) && s.Max > s.Min {
			yield(s.Max)
		}
		return
	}
	prev := math.Inf(-1)
	for k := k0; k <= last; k++ {
		v := min(max(k*s.Step, s.Min), s.Max)
		if v-s.Min <= s.Step*1e-9 {
			v = s.Min
		} else if s.Max-v <= s.Step*1e-9 {
			v = s.Max
		}
		v += 0 // turns -0 into 0
		if v <= prev || !yield(v) {
			return
		}
		prev = v
	}
}

// Label writes v as a tick label with the decimals Step needs and no more: on
// a 0.25 step, 0.5 is "0.50"; on a 50 step, 150 is "150". It never writes
// "-0", and writes NaN and infinities as "". Steps needing more than six
// decimals and magnitudes from 1e15 up, where plain digits get long or run
// past what a float64 holds, are written in exponent form, like "2.5e+20".
// Without a Step, v gets up to 15 significant digits.
func (s Scale) Label(v float64) string {
	switch {
	case !finite(v):
		return ""
	case !(s.Step > 0) || !finite(s.Step):
		return strconv.FormatFloat(v+0, 'g', 15, 64) // +0 turns -0 into 0
	}
	r := resolution(s.Step)
	if mag := max(math.Abs(s.Min), math.Abs(s.Max), math.Abs(v)); r < -6 || !(mag < 1e15) {
		if v == 0 {
			return "0"
		}
		e := int(math.Floor(math.Log10(math.Abs(v))))
		return strconv.FormatFloat(v, 'e', min(max(e-r, 0), 14), 64)
	}
	out := strconv.FormatFloat(v, 'f', max(-r, 0), 64)
	if out[0] == '-' && strings.Trim(out[1:], "0.") == "" {
		out = out[1:] // rounded to zero
	}
	return out
}

// resolution is the power of ten of step's last significant digit, the place
// a label must reach to tell multiples of step apart: -2 for 0.25, 1 for 50.
// It shrugs off float noise (0.30000000000000004 is 0.3) and stops at 15
// significant digits. Starting a place above Log10's estimate covers that
// estimate landing low on an exact power of ten.
func resolution(step float64) int {
	lead := int(math.Floor(math.Log10(step)))
	for r := lead + 1; r > lead-14; r-- {
		x := step / math.Pow(10, float64(r))
		if n := math.Round(x); n >= 1 && math.Abs(x-n) <= x*1e-9 {
			return r
		}
	}
	return lead - 14
}
