package decker

import (
	"math"
	"strconv"
	"strings"
)

// Chart components follow Panel's convention: a Rect in pixels, sizes from
// c.Unit and c.SmallText, colors from c.Theme, and a build step the chart
// grows in on. Before that step they draw nothing and return (0, 0). Bad data
// never panics: NaN and infinities count as missing, and an empty or all-zero
// chart draws its axes.

const (
	chartFade   = 0.35 // seconds for axes and labels to fade in
	chartGrow   = 0.7  // seconds for one bar to grow
	chartSweep  = 1.3  // seconds for a line to draw or a donut to sweep
	chartLead   = 0.15 // seconds the frame shows before the data starts
	statDefault = 1.2  // seconds a Stat counts when Duration is 0
	chartMinTxt = 6    // smallest size labels are shrunk to
)

// defaultSeries is the fallback palette for SeriesColor, so a theme that
// never sets Theme.Series still gets distinct, on-theme chart colors.
func (t *Theme) defaultSeries() [5]RGB {
	return [5]RGB{t.Accent, t.Accent2, t.Good, t.Warn, t.Muted}
}

// SeriesColor returns the color of chart series i, cycling through
// Theme.Series; when that is empty it cycles Accent, Accent2, Good, Warn and
// Muted. A negative i counts from the end of the cycle.
func (t *Theme) SeriesColor(i int) RGB {
	if len(t.Series) > 0 {
		return t.Series[((i%len(t.Series))+len(t.Series))%len(t.Series)]
	}
	d := t.defaultSeries()
	return d[((i%len(d))+len(d))%len(d)]
}

func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

func clean(v float64) float64 {
	if finite(v) {
		return v
	}
	return 0
}

// niceNum rounds x to 1, 2, 5 or 10 times a power of ten: the nearest when
// round is set, else the next one up. x must be positive and finite.
func niceNum(x float64, round bool) float64 {
	exp := math.Floor(math.Log10(x))
	f := x / math.Pow(10, exp)
	var nf float64
	switch {
	case round && f < 1.5, !round && f <= 1:
		nf = 1
	case round && f < 3, !round && f <= 2:
		nf = 2
	case round && f < 7, !round && f <= 5:
		nf = 5
	default:
		nf = 10
	}
	return nf * math.Pow(10, exp)
}

// axisScale is a value axis: the range it spans and the distance between
// gridlines.
type axisScale struct{ lo, hi, step float64 }

// niceScale returns an axis covering [lo, hi] with at most about maxTicks
// round-numbered gridlines, its ends moved out to multiples of the step.
func niceScale(lo, hi float64, maxTicks int) axisScale {
	if !(hi > lo) { // no spread: all zero draws 0..1, a constant is centered
		if lo == 0 {
			lo, hi = 0, 1
		} else {
			span := math.Abs(lo) / 2
			lo, hi = max(lo-span, -math.MaxFloat64), min(lo+span, math.MaxFloat64)
			if !(hi > lo) {
				lo, hi = math.Nextafter(lo, math.Inf(-1)), math.Nextafter(hi, math.Inf(1))
			}
		}
	}
	step := niceNum(niceNum(hi-lo, false)/float64(max(maxTicks-1, 1)), true)
	a := axisScale{
		lo:   math.Floor(lo/step+1e-9) * step,
		hi:   math.Ceil(hi/step-1e-9) * step,
		step: step,
	}
	if !finite(a.lo) || !finite(a.hi) || !finite(step) || !(step > 0) || !(a.hi > a.lo) {
		return extremeScale(lo, hi, maxTicks)
	}
	return a
}

// extremeScale keeps finite data visible when decimal rounding overflows or
// collapses a subnormal range. Retaining the endpoints beats losing the axis.
func extremeScale(lo, hi float64, maxTicks int) axisScale {
	span := hi - lo
	if !finite(span) {
		span = hi/2 - lo/2
	}
	step := max(span/float64(max(maxTicks-1, 1)), math.SmallestNonzeroFloat64)
	return axisScale{lo, hi, step}
}

// fixedScale is niceScale for a caller-chosen range: the ends stay put and
// only the gridline step is rounded.
func fixedScale(lo, hi float64, maxTicks int) axisScale {
	if !(hi > lo) {
		return niceScale(lo, hi, maxTicks)
	}
	step := niceNum(niceNum(hi-lo, false)/float64(max(maxTicks-1, 1)), true)
	if !finite(step) || !(step > 0) {
		return extremeScale(lo, hi, maxTicks)
	}
	return axisScale{lo, hi, step}
}

// ticks returns the gridline values, multiples of the step inside the range.
func (a axisScale) ticks() []float64 {
	if !(a.step > 0) {
		return []float64{a.lo, a.hi}
	}
	var out []float64
	for k := math.Ceil(a.lo/a.step - 1e-9); k <= math.Floor(a.hi/a.step+1e-9) && len(out) < 64; k++ {
		v := k*a.step + 0 // +0 turns -0 into 0
		if !finite(v) || k+1 == k {
			return []float64{a.lo, a.hi} // the step is below the range's precision
		}
		if len(out) > 0 && v <= out[len(out)-1] {
			break
		}
		out = append(out, v)
	}
	return out
}

// valueAxis is a scale with its gridline labels written out.
type valueAxis struct {
	axisScale
	ticks []float64
	text  []string
	w     float64 // the widest label
}

// newValueAxis scales [lo, hi] with about maxTicks gridlines, keeping hi as
// given if fixed.
func newValueAxis(f *Font, size int, format func(float64) string, lo, hi float64, fixed bool, maxTicks int) valueAxis {
	var a valueAxis
	if fixed {
		a.axisScale = fixedScale(lo, hi, maxTicks)
	} else {
		a.axisScale = niceScale(lo, hi, maxTicks)
	}
	a.ticks = a.axisScale.ticks()
	a.text = make([]string, len(a.ticks))
	for i, v := range a.ticks {
		a.text[i] = format(v)
		a.w = max(a.w, f.Measure(a.text[i], size))
	}
	return a
}

// fitValueAxis is newValueAxis with as many gridlines (up to six) as leave
// their labels clear of each other along room pixels: a line of text apart
// for a vertical axis, a label's width for a horizontal one.
func fitValueAxis(f *Font, size int, format func(float64) string, lo, hi float64, fixed bool, room float64, horizontal bool) valueAxis {
	for n := 6; ; n-- {
		a := newValueAxis(f, size, format, lo, hi, fixed, n)
		need := float64(size) * 1.8
		if horizontal {
			need = a.w + float64(size)
		}
		if n <= 2 || room*a.step/(a.hi-a.lo) >= need {
			return a
		}
	}
}

// at maps a value to 0 (lo) .. 1 (hi), clamped.
func (a axisScale) at(v float64) float64 {
	if !(a.hi > a.lo) {
		return 0
	}
	if v <= a.lo {
		return 0
	}
	if v >= a.hi {
		return 1
	}
	if math.IsInf(a.hi-a.lo, 0) {
		// Finite endpoints can span more than MaxFloat64. Halving before
		// subtracting keeps both the span and the relative position finite.
		return Clamp01((v/2 - a.lo/2) / (a.hi/2 - a.lo/2))
	}
	return Clamp01((v - a.lo) / (a.hi - a.lo))
}

// defaultFormat shortens large numbers (12.5k, 3M) and trims trailing zeros.
func defaultFormat(v float64) string {
	a := math.Abs(v)
	suffix, div := "", 1.0
	switch {
	case a >= 1e9:
		suffix, div = "B", 1e9
	case a >= 1e6:
		suffix, div = "M", 1e6
	case a >= 1e4:
		suffix, div = "k", 1e3
	}
	x := v / div
	places := 1e2
	if ax := math.Abs(x); ax != 0 && ax < 0.1 {
		places = 1e4
	}
	x = math.Round(x*places) / places
	if x == 0 {
		x = 0 // not "-0"
	}
	return strconv.FormatFloat(x, 'f', -1, 64) + suffix
}

func formatOr(f func(float64) string) func(float64) string {
	if f == nil {
		return defaultFormat
	}
	return f
}

// dataRange returns the smallest and largest finite value across series.
func dataRange(series [][]float64) (lo, hi float64, ok bool) {
	for _, s := range series {
		for _, v := range s {
			if !finite(v) {
				continue
			}
			if !ok {
				lo, hi, ok = v, v, true
			}
			lo, hi = min(lo, v), max(hi, v)
		}
	}
	return lo, hi, ok
}

func longest(series [][]float64) int {
	n := 0
	for _, s := range series {
		n = max(n, len(s))
	}
	return n
}

func fadeFX(a float64) GlyphEffect {
	return func(int) GlyphFX { return GlyphFX{Alpha: a} }
}

// chartText draws one line of s with its top at y, fading in at alpha a.
func chartText(c Ctx, p *Pixels, s string, size int, col RGB, align Align, x, y, a float64) (w, h float64) {
	if a <= 0 || s == "" {
		return 0, 0
	}
	t := Text{Font: c.Theme.Body, Size: size, Color: col, Align: align}
	if a < 1 {
		t.FX = fadeFX(a)
	}
	return t.Draw(p, s, x, y)
}

// chartTextMid is chartText with the ink centered on cy.
func chartTextMid(c Ctx, p *Pixels, s string, size int, col RGB, align Align, x, cy, a float64) {
	if a <= 0 || s == "" {
		return
	}
	t := Text{Font: c.Theme.Body, Size: size, Color: col, Align: align}
	if a < 1 {
		t.FX = fadeFX(a)
	}
	t.DrawMid(p, s, x, cy)
}

// catLayout is category labels wrapped, and shrunk if need be, to a slot.
// Only every stride-th label is drawn; the rest have no lines.
type catLayout struct {
	size   int
	stride int
	lines  [][]string
	w, h   float64 // the widest line and the tallest label
}

// labelsKey encodes lengths so embedded separators cannot alias another
// label list, including no labels versus one empty label.
func labelsKey(labels []string) string {
	var b strings.Builder
	for _, label := range labels {
		b.WriteString(strconv.Itoa(len(label)))
		b.WriteByte(':')
		b.WriteString(label)
	}
	return b.String()
}

type catKey struct {
	f          *Font
	labels     string
	maxW, maxH float64
	size       int
	skip       bool
}

var catLayouts = memo[catKey, catLayout]{max: 2000}

// layoutCats wraps each label to maxW and shrinks the whole set, down to
// chartMinTxt, until every label is at most maxH tall and none has a word
// wider than maxW. With skip, a set that would shrink below four fifths of
// base instead draws every second, third, ... label, each given that many
// slots. The result is shared and must not be modified.
func layoutCats(f *Font, labels []string, maxW, maxH float64, base int, skip bool) catLayout {
	return catLayouts.get(catKey{f, labelsKey(labels), maxW, maxH, base, skip}, func() catLayout {
		for stride := 1; ; stride++ {
			out := layoutCatsAt(f, labels, maxW*float64(stride), maxH, base, stride)
			if !skip || out.size*5 >= base*4 || stride >= len(labels) {
				return out
			}
		}
	})
}

func layoutCatsAt(f *Font, labels []string, maxW, maxH float64, base, stride int) catLayout {
	var out catLayout
	largestSize(base, chartMinTxt, func(size int) bool {
		out = catLayout{size: size, stride: stride, lines: make([][]string, len(labels))}
		ok := true
		for i := 0; i < len(labels); i += stride {
			out.lines[i] = f.Wrap(labels[i], size, maxW)
			for _, ln := range out.lines[i] {
				m := f.Measure(ln, size)
				out.w = max(out.w, m)
				ok = ok && m <= maxW
			}
			h := float64(len(out.lines[i])) * float64(size) * DefaultLeading
			out.h = max(out.h, h)
			ok = ok && h <= maxH
		}
		return ok
	})
	return out
}

// legendRow draws a one-line key of swatches and names across the top of r,
// as many as fit. A name missing from names is "Series N".
func legendRow(c Ctx, p *Pixels, r Rect, names []string, count int, a float64) {
	th := c.Theme
	size := c.SmallText(th.Body)
	sw := float64(size) * 0.7
	gap := float64(size) * 0.9
	x := r.X
	if r.H < float64(size)*DefaultLeading {
		return
	}
	for i := 0; i < count; i++ {
		name := "Series " + strconv.Itoa(i+1)
		if i < len(names) {
			name = names[i]
		}
		if x+sw*1.4+th.Body.Measure(name, size) > r.Right() {
			return
		}
		p.RoundRect(x, r.Y+(float64(size)*DefaultLeading-sw)/2, sw, sw, sw*0.25, 0, th.SeriesColor(i), a)
		w, _ := chartText(c, p, name, size, th.Muted, Left, x+sw*1.4, r.Y, a)
		x += sw*1.4 + w + gap
	}
}

// BarChart is a bar chart that grows from its baseline: bars rise (or extend,
// if Horizontal) one after another, then value labels appear on their ends.
// Set Values for one series, or Series for grouped bars; Names then label a
// legend.
//
// The value axis has a few gridlines at round numbers. Negative values hang
// below a zero line, and NaN or infinite values leave a gap. Category labels
// are wrapped, then shrunk, to fit their slot.
type BarChart struct {
	Labels     []string    // one per category
	Values     []float64   // one series; ignored if Series is set
	Series     [][]float64 // grouped bars: one slice of values per series
	Names      []string    // series names for the legend, shown for two or more series
	Max        float64     // the axis top; 0 picks one that rounds up from the data
	Horizontal bool        // bars run left to right, with labels down the left side
	ShowValues bool        // write each value at the end of its bar
	Step       int         // the build step it grows in on
	Stagger    float64     // seconds between bars starting; 0 is 0.08, negative is none
	Format     func(float64) string
}

func (b BarChart) series() [][]float64 {
	if len(b.Series) > 0 {
		return b.Series
	}
	return [][]float64{b.Values}
}

// Draw renders the chart into r and returns the size it drew, which is r's.
func (b BarChart) Draw(c Ctx, p *Pixels, r Rect) (w, h float64) {
	since := c.Since(b.Step)
	if since < 0 || r.W <= 0 || r.H <= 0 {
		return 0, 0
	}
	defer c.within("BarChart", r)()
	th := c.Theme
	size := c.SmallText(th.Body)
	series := b.series()
	k := len(series)
	n := max(len(b.Labels), longest(series))
	format := formatOr(b.Format)
	frame := Ease(since, chartFade)
	gap := c.Unit(0.02)
	fs := float64(size)
	lineH := fs * DefaultLeading

	dlo, dhi, _ := dataRange(series)
	lo, hi := min(dlo, 0), max(dhi, 0)
	fixed := b.Max > 0 && finite(b.Max)
	if fixed {
		hi = b.Max
	}
	// Labels are widest at about five gridlines; the final count waits for the
	// plot's size.
	tickW := newValueAxis(th.Body, size, format, lo, hi, fixed, 5).w

	top := fs * 0.6
	if k > 1 {
		top = lineH + gap
		legendRow(c, p, Rect{r.X, r.Y, r.W, min(lineH, r.H)}, b.Names, k, frame)
	}
	// Room for a value label past the end of the longest bar, and past the end
	// of the longest negative bar.
	valRoom := 0.0
	if b.ShowValues {
		valRoom = lineH + gap/2
	}

	var plot Rect
	var cats catLayout
	var catRight float64 // where horizontal category labels end
	loRoom := 0.0        // below the plot, for the labels of negative bars
	if b.Horizontal {
		cats = layoutCats(th.Body, b.Labels, r.W*0.35, r.H/float64(max(n, 1))*0.95, size, false)
		right := fs * 1.5 // half a tick label, and a little
		if b.ShowValues {
			right = r.W * 0.08
			for _, s := range series {
				for _, v := range s {
					if finite(v) {
						right = max(right, th.Body.Measure(format(v), size)+gap)
					}
				}
			}
		}
		left := 0.0
		if cats.w > 0 {
			left = cats.w + gap
		}
		catRight = r.X + cats.w
		// A negative bar's label sits left of the bar, so it gets a gutter
		// between the category labels and the plot.
		if b.ShowValues {
			gutter := 0.0
			for _, s := range series {
				for _, v := range s {
					if finite(v) && v < 0 {
						gutter = max(gutter, th.Body.Measure(format(v), size)+gap/2)
					}
				}
			}
			left += gutter
		}
		plot = Rect{r.X + left, r.Y + top, max(r.W-left-right, 0), max(r.H-top-lineH-gap, 0)}
	} else {
		left := tickW + gap
		plot = Rect{r.X + left, r.Y + top, max(r.W-left, 0), 0}
		cats = layoutCats(th.Body, b.Labels, plot.W/float64(max(n, 1))*0.92, r.H*0.25, size, false)
		catH := 0.0
		if cats.h > 0 {
			catH = cats.h + gap
		}
		hiRoom := valRoom
		if lo < 0 {
			loRoom = valRoom
		}
		plot.Y += hiRoom
		plot.H = max(r.H-top-hiRoom-loRoom-catH, 0)
	}
	if plot.W <= 0 || plot.H <= 0 {
		return r.W, r.H
	}

	room := plot.H
	if b.Horizontal {
		room = plot.W
	}
	ax := fitValueAxis(th.Body, size, format, lo, hi, fixed, room, b.Horizontal)
	sc, ticks, tickText := ax.axisScale, ax.ticks, ax.text

	// pos maps a value to a coordinate along the value axis.
	pos := func(v float64) float64 {
		if b.Horizontal {
			return plot.X + sc.at(v)*plot.W
		}
		return plot.Bottom() - sc.at(v)*plot.H
	}
	grid := max(c.Unit(0.005), 1)
	for i, v := range ticks {
		g := pos(v)
		if b.Horizontal {
			p.Rect(g-grid/2, plot.Y, grid, plot.H, th.Faint, frame)
			chartText(c, p, tickText[i], size, th.Muted, Center, g, plot.Bottom()+gap/2, frame)
		} else {
			p.Rect(plot.X, g-grid/2, plot.W, grid, th.Faint, frame)
			chartTextMid(c, p, tickText[i], size, th.Muted, Right, plot.X-gap, g, frame)
		}
	}
	zero := pos(0)
	if b.Horizontal {
		p.Rect(zero-grid/2, plot.Y, grid, plot.H, th.Muted, frame*0.7)
	} else {
		p.Rect(plot.X, zero-grid/2, plot.W, grid, th.Muted, frame*0.7)
	}

	slot := plot.W / float64(max(n, 1))
	if b.Horizontal {
		slot = plot.H / float64(max(n, 1))
	}
	for i := 0; i < n; i++ {
		if i >= len(cats.lines) {
			break
		}
		txt := strings.Join(cats.lines[i], "\n")
		mid := (float64(i) + 0.5) * slot
		if b.Horizontal {
			lh := float64(len(cats.lines[i])) * float64(cats.size) * DefaultLeading
			chartText(c, p, txt, cats.size, th.Muted, Right, catRight, plot.Y+mid-lh/2, frame)
		} else {
			chartText(c, p, txt, cats.size, th.Muted, Center, plot.X+mid, plot.Bottom()+loRoom+gap, frame)
		}
	}

	// The bars. A group fills 70% of its slot, so bars never touch the next
	// group's.
	group := slot * 0.7
	bar := min(group/(float64(k)+float64(k-1)*0.12), c.Unit(0.4))
	group = bar * (float64(k) + float64(k-1)*0.12)
	stagger := b.Stagger
	if stagger == 0 {
		stagger = 0.08
	}
	stagger = min(max(stagger, 0), 1.0/float64(max(n*k-1, 1)))
	radius := min(bar/2, c.Unit(0.01))
	// A value label may be as wide as its bar and the gap to the next bar, so
	// that neighbors in a group don't run together.
	lim := bar * 1.12
	if k == 1 {
		lim = slot * 0.95
	}
	for i := 0; i < n; i++ {
		for j, s := range series {
			if i < 0 || i >= len(s) || !finite(s[i]) {
				continue
			}
			v := s[i]
			t := since - chartLead - float64(i*k+j)*stagger
			grown := Ease(t, chartGrow)
			if grown <= 0 {
				continue
			}
			end := pos(clampTo(sc, v)) // the clamped value's end
			tip := Lerp(zero, end, grown)
			off := (slot-group)/2 + float64(j)*bar*1.12
			col := th.SeriesColor(j)
			var rx, ry, rw, rh float64
			if b.Horizontal {
				rx, ry, rw, rh = math.Min(zero, tip), plot.Y+float64(i)*slot+off, math.Abs(tip-zero), bar
			} else {
				rx, ry, rw, rh = plot.X+float64(i)*slot+off, math.Min(zero, tip), bar, math.Abs(tip-zero)
			}
			if rw > 0 && rh > 0 {
				p.RoundRect(rx, ry, rw, rh, radius, 0, col, 1)
			}
			if !b.ShowValues {
				continue
			}
			la := Progress(t, chartGrow*0.8, 0.25)
			if la <= 0 {
				continue
			}
			txt := format(v)
			ls := size
			if m := th.Body.Measure(txt, ls); !b.Horizontal && m > lim {
				ls = max(int(float64(ls)*lim/m), chartMinTxt)
			}
			if b.Horizontal {
				if v >= 0 {
					chartTextMid(c, p, txt, ls, th.Text, Left, rx+rw+gap/2, ry+bar/2, la)
				} else {
					chartTextMid(c, p, txt, ls, th.Text, Right, rx-gap/2, ry+bar/2, la)
				}
			} else if v >= 0 {
				chartText(c, p, txt, ls, th.Text, Center, rx+bar/2, ry-float64(ls)*DefaultLeading-2, la)
			} else {
				chartText(c, p, txt, ls, th.Text, Center, rx+bar/2, ry+rh+2, la)
			}
		}
	}
	return r.W, r.H
}

// clampTo clamps v to the axis so a bar past an explicit Max stops at the top.
func clampTo(a axisScale, v float64) float64 { return min(max(v, a.lo), a.hi) }

// LineChart draws one or more series as lines over shared category labels. The
// lines draw on from left to right as they ease in; Points marks each value.
// With more than one series a legend names them, using Names. NaN and
// infinite values break a line.
type LineChart struct {
	Labels []string    // x-axis categories
	Series [][]float64 // one slice of values per line
	Names  []string    // series names for the legend
	Max    float64     // the axis top; 0 fits the data
	Min    float64     // the axis bottom; 0 fits the data, down to zero if its smallest value is at most half its largest
	Step   int         // the build step it draws on at
	Points bool        // mark every value with a dot
	Format func(float64) string
}

// Draw renders the chart into r and returns the size it drew, which is r's.
func (l LineChart) Draw(c Ctx, p *Pixels, r Rect) (w, h float64) {
	since := c.Since(l.Step)
	if since < 0 || r.W <= 0 || r.H <= 0 {
		return 0, 0
	}
	defer c.within("LineChart", r)()
	th := c.Theme
	size := c.SmallText(th.Body)
	fs := float64(size)
	lineH := fs * DefaultLeading
	gap := c.Unit(0.02)
	n := max(len(l.Labels), longest(l.Series))
	format := formatOr(l.Format)
	frame := Ease(since, chartFade)

	lo, hi, _ := dataRange(l.Series)
	if l.Min != 0 && finite(l.Min) {
		lo = l.Min
	}
	if l.Max != 0 && finite(l.Max) {
		hi = l.Max
	}
	if l.Min == 0 && lo > 0 && lo <= hi/2 {
		lo = 0 // data that already spans most of the way down to zero reads better from it
	}
	fixed := (l.Min != 0 || l.Max != 0) && hi > lo
	tickW := newValueAxis(th.Body, size, format, lo, hi, fixed, 5).w

	top := fs * 0.6
	if len(l.Series) > 1 {
		top = lineH + gap
		legendRow(c, p, Rect{r.X, r.Y, r.W, min(lineH, r.H)}, l.Names, len(l.Series), frame)
	}
	left := tickW + gap
	slot := max(r.W-left, 0) / float64(max(n, 1))
	cats := layoutCats(th.Body, l.Labels, slot*0.92, r.H*0.25, size, true)
	catH := 0.0
	if cats.h > 0 {
		catH = cats.h + gap
	}
	plot := Rect{r.X + left, r.Y + top, max(r.W-left, 0), max(r.H-top-catH, 0)}
	if plot.W <= 0 || plot.H <= 0 {
		return r.W, r.H
	}

	ax := fitValueAxis(th.Body, size, format, lo, hi, fixed, plot.H, false)
	sc, ticks, tickText := ax.axisScale, ax.ticks, ax.text
	grid := max(c.Unit(0.005), 1)
	for i, v := range ticks {
		y := plot.Bottom() - sc.at(v)*plot.H
		p.Rect(plot.X, y-grid/2, plot.W, grid, th.Faint, frame)
		chartTextMid(c, p, tickText[i], size, th.Muted, Right, plot.X-gap, y, frame)
	}
	for i := 0; i < n && i < len(cats.lines); i += cats.stride {
		chartText(c, p, strings.Join(cats.lines[i], "\n"), cats.size, th.Muted, Center,
			plot.X+(float64(i)+0.5)*slot, plot.Bottom()+gap, frame)
	}

	reveal := plot.X + Ease(since-chartLead, chartSweep)*plot.W
	width := max(c.Unit(0.014), 2)
	for j, s := range l.Series {
		col := th.SeriesColor(j)
		pt := func(i int) (float64, float64, bool) {
			if i < 0 || i >= len(s) || !finite(s[i]) {
				return 0, 0, false
			}
			return plot.X + (float64(i)+0.5)*slot, plot.Bottom() - sc.at(s[i])*plot.H, true
		}
		for i := 0; i < len(s); i++ {
			x, y, ok := pt(i)
			if !ok || x > reveal {
				continue
			}
			if x2, y2, ok2 := pt(i + 1); ok2 {
				f := Clamp01((reveal - x) / (x2 - x))
				p.Line(x, y, Lerp(x, x2, f), Lerp(y, y2, f), width, col, 1)
			}
			_, _, hasPrev := pt(i - 1)
			_, _, hasNext := pt(i + 1)
			if l.Points || (!hasPrev && !hasNext) {
				// A lone value has no line to show it, so it always gets a dot.
				pop := EaseOutBack(Progress(reveal-x, 0, slot*0.5+1))
				p.Disc(x, y, width*1.6*pop, th.Background, 1)
				p.Disc(x, y, width*1.1*pop, col, 1)
			}
		}
	}
	return r.W, r.H
}

// DonutChart is a ring split into segments by value, sweeping in clockwise
// from 12 o'clock, with a legend beside it giving each label and its share.
// Negative, NaN and infinite values count as zero. Segments are drawn with
// round ends and a small gap, so they read as separate even when thin.
type DonutChart struct {
	Labels    []string
	Values    []float64
	Thickness float64 // the ring's width as a fraction of its radius, 0 is 0.32
	Center    string  // big text in the hole
	Step      int     // the build step it sweeps in on
}

type legendKey struct {
	f           *Font
	labels      string
	avail, room float64
	max, min    int
	shares      bool
}

type legendLayout struct {
	size  int
	lines [][]string
	w, h  float64 // content width and height
	pct   float64 // width of the share column
}

var legendLayouts = memo[legendKey, legendLayout]{max: 1000}

// layoutLegend picks the largest size in [minSize, maxSize] at which the
// labels, wrapped beside a swatch and a share column, fit avail x room.
func layoutLegend(f *Font, labels []string, avail, room float64, maxSize, minSize int, shares bool) legendLayout {
	return legendLayouts.get(legendKey{f, labelsKey(labels), avail, room, maxSize, minSize, shares}, func() legendLayout {
		var out legendLayout
		largestSize(maxSize, minSize, func(size int) bool {
			fs := float64(size)
			pct := 0.0
			if shares {
				pct = f.Measure("100%", size)
			}
			labelW := max(avail-fs*0.7-fs*0.7-fs*0.9-pct, fs)
			out = legendLayout{size: size, lines: make([][]string, len(labels)), pct: pct}
			for i, l := range labels {
				out.lines[i] = f.Wrap(l, size, labelW)
				lw := 0.0
				for _, ln := range out.lines[i] {
					lw = max(lw, f.Measure(ln, size))
				}
				out.w = max(out.w, lw)
				out.h += float64(len(out.lines[i])) * fs * DefaultLeading
			}
			out.h += float64(max(len(labels)-1, 0)) * fs * 0.45
			out.w += fs*0.7*2 + fs*0.9 + pct
			return out.h <= room && out.w <= avail
		})
		return out
	})
}

type donutKey struct {
	f         *Font
	labels    string
	w, h, gap float64
	max, min  int
}

type donutLayout struct {
	diam float64
	lay  legendLayout
}

var donutLayouts = memo[donutKey, donutLayout]{max: 1000}

// layoutDonut sizes the ring and its legend to share w x h: the ring is as
// big as it can be while the legend still fits beside it.
func layoutDonut(f *Font, labels []string, w, h, gap float64, maxSize, minSize int) (float64, legendLayout) {
	k := donutKey{f, labelsKey(labels), w, h, gap, maxSize, minSize}
	r := donutLayouts.get(k, func() donutLayout {
		if len(labels) == 0 {
			return donutLayout{diam: min(h, w)}
		}
		// Shrink the ring to make room for the legend, but not below 70%; past
		// that, drop the share column.
		full := min(h, w*0.6)
		var out donutLayout
		for _, shares := range []bool{true, false} {
			for f2 := 1.0; f2 >= 0.7-1e-9; f2 -= 0.05 {
				out.diam = full * f2
				out.lay = layoutLegend(f, labels, max(w-out.diam-gap, 0), h, maxSize, minSize, shares)
				if out.lay.w <= w-out.diam-gap && out.lay.h <= h {
					return out
				}
			}
		}
		return donutLayout{diam: min(h, w)} // no room for a legend: ring alone
	})
	return r.diam, r.lay
}

// Draw renders the chart into r and returns the size it drew: the ring and its
// legend, centered in r.
func (d DonutChart) Draw(c Ctx, p *Pixels, r Rect) (w, h float64) {
	since := c.Since(d.Step)
	if since < 0 || r.W <= 0 || r.H <= 0 {
		return 0, 0
	}
	defer c.within("DonutChart", r)()
	th := c.Theme
	n := len(d.Labels)
	if len(d.Values) > n {
		n = len(d.Values)
	}
	vals := make([]float64, n)
	total, nonzero := 0.0, 0
	for i := range vals {
		if i < len(d.Values) && finite(d.Values[i]) && d.Values[i] > 0 {
			vals[i] = d.Values[i]
			total += vals[i]
			nonzero++
		}
	}
	labels := make([]string, n)
	copy(labels, d.Labels)

	gap := c.Unit(0.06)
	diam, lay := layoutDonut(th.Body, labels, r.W, r.H, gap, th.Body.Drawn(c.Size(0.075)), c.SmallText(th.Body))
	legendW := lay.w
	hasLegend := lay.size > 0
	if !hasLegend {
		gap = 0
	}
	groupX := r.X + (r.W-diam-gap-legendW)/2
	cx, cy := groupX+diam/2, r.Y+r.H/2
	outer := diam / 2
	thick := outer * min(max(d.Thickness, 0.05), 1)
	if d.Thickness == 0 {
		thick = outer * 0.32
	}
	rm := outer - thick/2
	if rm <= 0 {
		return r.W, r.H
	}

	frame := Ease(since, chartFade)
	p.Arc(cx, cy, rm, thick, 0, 2*math.Pi, th.Faint, 0.35*frame)
	sweep := 2 * math.Pi * Ease(since-chartLead, chartSweep)

	// Each end is a round cap reaching half the ring's width past it, so pull
	// the ends in by that plus half the gap between neighbors.
	inset := (thick/2 + max(c.Unit(0.008), 1)/2) / rm
	if nonzero < 2 {
		inset = 0
	}
	start := 0.0
	for i, v := range vals {
		if v <= 0 {
			continue
		}
		end := start + v/total*2*math.Pi
		a0, a1 := start+inset, end-inset
		if a1 <= a0 {
			a0 = (start + end) / 2
			a1 = a0 + 1e-4
		}
		a1 = min(a1, sweep)
		if a1 > a0 {
			p.Arc(cx, cy, rm, thick, a0, a1, th.SeriesColor(i), 1)
		}
		start = end
	}

	if d.Center != "" && outer-thick > 0 {
		inner := (outer - thick) * 2 * 0.8
		size, txt := th.Display.Fit(d.Center, inner, inner*0.6, c.Size(0.25), 0)
		lines := float64(strings.Count(txt, "\n") + 1)
		Text{Font: th.Display, Size: size, Color: th.Text, Align: Center, FX: fadeFX(Ease(since-0.3, 0.5))}.
			Draw(p, txt, cx, cy-lines*float64(size)*DefaultLeading/2)
	}

	if hasLegend {
		size := lay.size
		fs := float64(size)
		sw := fs * 0.7
		lx := cx + outer + gap
		y := cy - lay.h/2
		for i := range n {
			a := Ease(since-chartLead-0.08*float64(i), chartFade)
			lh := float64(len(lay.lines[i])) * fs * DefaultLeading
			p.RoundRect(lx, y+(fs*DefaultLeading-sw)/2, sw, sw, sw*0.25, 0, th.SeriesColor(i), a)
			chartText(c, p, strings.Join(lay.lines[i], "\n"), size, th.Text, Left, lx+sw*1.7, y, a)
			if lay.pct > 0 {
				share := "0%"
				if total > 0 {
					share = strconv.Itoa(int(math.Round(vals[i]/total*100))) + "%"
				}
				chartText(c, p, share, size, th.Muted, Right, lx+legendW, y, a)
			}
			y += lh + fs*0.45
		}
	}
	return diam + gap + legendW, max(diam, lay.h)
}

// Sparkline draws values as a tiny line chart with no axes inside r, scaled to
// fit them, in col. prog (0..1) is how much of the line to draw, left to
// right, so Ease(c.Since(step), 0.8) animates it; a dot marks the end of what
// is drawn. NaN values break the line, and a constant series is a flat line.
func Sparkline(c Ctx, p *Pixels, r Rect, values []float64, col RGB, prog float64) {
	prog = Clamp01(prog)
	if prog <= 0 || len(values) == 0 || r.W <= 0 || r.H <= 0 {
		return
	}
	lo, hi, ok := dataRange([][]float64{values})
	if !ok {
		return
	}
	// The stroke and dot scale with the canvas, but never past what r can hold.
	width := min(max(c.Unit(0.01), 1.5), min(r.W, r.H)/3)
	dot := min(width*1.5, min(r.W, r.H)/2)
	box := r.Inset(dot, dot)
	x := func(i int) float64 {
		if len(values) == 1 {
			return box.X + box.W/2
		}
		return box.X + box.W*float64(i)/float64(len(values)-1)
	}
	y := func(i int) float64 {
		if hi <= lo {
			return box.Y + box.H/2
		}
		return box.Bottom() - (values[i]-lo)/(hi-lo)*box.H
	}
	reveal := box.X + prog*box.W
	hx, hy, have := 0.0, 0.0, false
	for i, v := range values {
		if !finite(v) || (len(values) > 1 && x(i) > reveal) {
			continue
		}
		hx, hy, have = x(i), y(i), true
		if i+1 < len(values) && finite(values[i+1]) {
			f := 1.0
			if dx := x(i+1) - x(i); dx > 0 {
				f = Clamp01((reveal - x(i)) / dx)
			}
			hx, hy = Lerp(x(i), x(i+1), f), Lerp(y(i), y(i+1), f)
			p.Line(x(i), y(i), hx, hy, width, col, 1)
		}
	}
	if have {
		p.Disc(hx, hy, dot, col, 1)
	}
}

// Stat is a big number that counts up from zero to Value when its step begins,
// with a small Muted label under it. The number is drawn in Display and the
// theme's Accent at the largest size that fits, with every digit in a cell as
// wide as the widest, and right-aligned in the width of the final value, so
// nothing shifts while it counts.
type Stat struct {
	Value    float64
	Prefix   string  // before the number: "$"
	Suffix   string  // after it: "%", " ms"
	Decimals int     // digits after the point
	Label    string  // what the number is, under it
	Step     int     // the build step it counts up on
	Duration float64 // seconds to count; 0 is 1.2
}

func (s Stat) duration() float64 {
	if s.Duration > 0 {
		return s.Duration
	}
	return statDefault
}

// text returns what the stat reads since seconds after its step began.
func (s Stat) text(since float64) string {
	d := min(max(s.Decimals, 0), 10)
	v := clean(s.Value) * Ease(since, s.duration())
	if m := math.Pow(10, float64(d)); math.Round(v*m) == 0 {
		v = 0 // not "-0.0"
	}
	return s.Prefix + strconv.FormatFloat(v, 'f', d, 64) + s.Suffix
}

// tabular lays s out with each digit centered in a cell as wide as the widest
// digit. It returns each glyph's shift from where plain text would put it, and
// the laid-out width.
func tabular(f *Font, size int, s string) (dx []float64, w float64) {
	f, size = f.resolve(size)
	cell := 0.0
	for d := '0'; d <= '9'; d++ {
		cell = max(cell, f.glyph(d, size).adv)
	}
	dx = make([]float64, 0, len(s))
	pen, prev := 0.0, rune(-1)
	for _, r := range s {
		if prev >= 0 {
			pen += f.kern(prev, r, size)
		}
		adv := f.glyph(r, size).adv
		if r >= '0' && r <= '9' {
			dx = append(dx, w+(cell-adv)/2-pen)
			w += cell
		} else {
			dx = append(dx, w-pen)
			w += adv
		}
		pen += adv
		prev = r
	}
	return dx, w
}

type statKey struct {
	f          *Font
	text       string
	maxW, maxH float64
	maxSize    int
}

var statSizes = memo[statKey, int]{max: 500}

// statSize is the largest size at which text's digits are at most maxH tall
// and its tabular width is at most maxW.
func statSize(f *Font, text string, maxW, maxH float64, maxSize int) int {
	return statSizes.get(statKey{f, text, maxW, maxH, maxSize}, func() int {
		return largestSize(maxSize, minFitSize, func(size int) bool {
			rf, rs := f.resolve(size)
			_, w := tabular(f, size, text)
			return w <= maxW && rf.CapHeight(rs) <= maxH
		})
	})
}

// Draw renders the stat centered in r and returns the size it drew.
func (s Stat) Draw(c Ctx, p *Pixels, r Rect) (w, h float64) {
	since := c.Since(s.Step)
	if since < 0 || r.W <= 0 || r.H <= 0 {
		return 0, 0
	}
	defer c.within("Stat", r)()
	th := c.Theme
	ls := c.SmallText(th.Body)
	gap := c.Unit(0.02)
	var labelLines []string
	labelH := 0.0
	if s.Label != "" {
		labelLines = th.Body.Wrap(s.Label, ls, r.W)
		labelH = float64(len(labelLines))*float64(ls)*DefaultLeading + gap
	}
	final := s.text(Settled)
	size := statSize(th.Display, final, r.W, max(r.H-labelH, 0), c.Size(0.5))
	_, finalW := tabular(th.Display, size, final)
	rf, rs := th.Display.resolve(size)
	capH := rf.CapHeight(rs)

	cur := s.text(since)
	dx, curW := tabular(th.Display, size, cur)
	fade := Ease(since, 0.3)
	num := Text{Font: th.Display, Size: size, Color: th.Accent,
		FX: func(i int) GlyphFX { return GlyphFX{DX: dx[i], Alpha: fade} }}
	total := capH + labelH
	y0 := r.Y + (r.H-total)/2
	right := r.X + (r.W+finalW)/2
	num.Draw(p, cur, right-curW, y0+capH-num.Baseline())
	if len(labelLines) > 0 {
		chartText(c, p, strings.Join(labelLines, "\n"), ls, th.Muted, Center, r.X+r.W/2, y0+capH+gap, fade)
	}
	return finalW, total
}
