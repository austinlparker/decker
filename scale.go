package decker

// Scale maps data values onto a span of pixels: the axis of a visual drawn by
// hand, like a trace waterfall's milliseconds. It draws nothing. At places a
// value, Ticks lists round values to mark and Label writes them:
//
//	ms := NiceScale(0, trace.Dur, 6, plot.X, plot.Right())
//	for _, v := range ms.Ticks() {
//		x := ms.At(v)
//		p.Rect(x, plot.Bottom(), 1, c.Unit(0.01), c.Theme.Faint, 1)
//		Text{Font: c.Theme.Body, Size: c.SmallText(c.Theme.Body), Color: c.Theme.Muted, Align: Center}.
//			Draw(p, ms.Label(v)+"ms", x, plot.Bottom()+c.Unit(0.015))
//	}
//
// Its ticks and labels are the charts' gridlines and axis labels, so a
// hand-drawn axis agrees with a BarChart beside it. A Scale is a plain value:
// build one by hand, or adjust what NiceScale returns.
type Scale struct {
	Min, Max float64 // the data range shown; NiceScale rounds it out to whole steps
	Step     float64 // the distance between ticks
	From, To float64 // the pixel positions of Min and Max; To < From runs upward, for a y axis
}

// NiceScale returns a Scale covering lo..hi with about maxTicks ticks at
// round numbers (1, 2 or 5 times a power of ten), its ends moved out to
// whole steps, mapped onto the pixels from..to. A range of one value is
// widened around it. To keep the data's own ends, say a trace that ran
// 347ms, set Min and Max back afterwards: the ticks stay round.
func NiceScale(lo, hi float64, maxTicks int, from, to float64) Scale {
	a := niceScale(min(lo, hi), max(lo, hi), maxTicks)
	return Scale{Min: a.lo, Max: a.hi, Step: a.step, From: from, To: to}
}

// At returns the pixel position of v: From at Min, To at Max, in proportion
// between and beyond them. A scale with no range puts every value at From.
func (s Scale) At(v float64) float64 {
	if !(s.Max > s.Min) {
		return s.From
	}
	u := (v - s.Min) / (s.Max - s.Min)
	return s.From + (s.To-s.From)*u
}

// Ticks returns the tick values, the multiples of Step from Min to Max, as a
// chart places its gridlines.
func (s Scale) Ticks() []float64 { return axisScale{s.Min, s.Max, s.Step}.ticks() }

// Label writes a tick value as a chart's axis does: trailing zeros trimmed,
// large values shortened (12.5k, 3M).
func (s Scale) Label(v float64) string { return defaultFormat(v) }
