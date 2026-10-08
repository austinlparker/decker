package main

// Recipe: a trace waterfall.
//
// What it shows: one request's spans as bars on a shared millisecond axis,
// each child indented under its parent and colored by its service, one span
// per build, and on the last build the critical path picked out: the chain
// of spans that decided when the request finished.
//
// How it's built:
//
//  1. Layout. layoutWaterfall is a pure function of the rect and the spans.
//     It measures the names with Text.Measure to size the left column,
//     builds the axis with NiceScale, cuts a row per span, decides where
//     each duration label goes (inside its bar when it fits there, else
//     after the bar, else before it) and says how much room it all needs.
//     Nothing is drawn yet.
//  2. Check. The slide hands that size to c.Fits. When it doesn't fit at a
//     readable size, the slide asks for a squeezed layout instead: no
//     legend, only the spans that fit the height (the tail is dropped), and
//     names shortened with "…" to a narrower column. It never shrinks the
//     text or lets it clip, and the review reports the squeeze, so you find
//     out before the audience does.
//  3. Draw. drawWaterfall paints gridlines and tick labels, then each span's
//     tree guide, name, bar and duration label, then the legend.
//  4. Reveal. Span i appears on build i, its bar growing from its start
//     time; the build after the last span dims everything off the critical
//     path.
//
// Knobs: the trace (your spans, in tree order: each parent before its
// children), the services list (one color each, from Theme.Series), the bar
// thickness (barFrac) and the reveal timing in drawWaterfall.

import (
	"strconv"

	"github.com/austinlparker/decker"
)

// span is one timed operation in a trace.
type span struct {
	name    string
	service string  // picks the bar's color; see services
	parent  int     // index of the parent span; -1 for the root
	start   float64 // ms since the trace began
	dur     float64 // ms
}

// checkoutTrace is the trace the slide draws: a checkout whose payment call
// dominates.
var checkoutTrace = []span{
	{"GET /checkout", "web", -1, 0, 418},
	{"auth.verify", "auth", 0, 6, 38},
	{"cart.load", "cart", 0, 50, 96},
	{"SELECT items", "db", 2, 61, 71},
	{"charge", "pay", 0, 152, 252},
	{"stripe.post", "pay", 4, 168, 221},
}

// services lists the services in legend order; the i-th takes
// Theme.SeriesColor(i).
var services = []string{"web", "auth", "cart", "db", "pay"}

// barFrac is a bar's thickness as a share of its row.
const barFrac = 0.62

func waterfallSlide() decker.Slide {
	trace := checkoutTrace
	return decker.Slide{Title: "Trace waterfall", Section: "Waterfall", Transition: decker.TransitionPush,
		Steps: len(trace) + 1, // a build per span, then one for the critical path
		Notes: "One span per click. The last click dims everything off the critical path: the payment call is where the time goes.",
		View: func(c decker.Ctx, sc *decker.Scene) {
			area := heading(c, sc.Px, "Trace waterfall")
			lay := layoutWaterfall(c, area, trace, false)
			if !c.Fits("waterfall", area, lay.needW, lay.needH) {
				lay = layoutWaterfall(c, area, trace, true)
			}
			drawWaterfall(c, sc.Px, lay, len(trace))
		}}
}

// waterfallLayout is where everything goes, worked out before drawing.
type waterfallLayout struct {
	spans  []span        // the spans shown: all of them unless squeezed
	text   decker.Text   // one size for names, ticks, labels and legend
	lineH  float64       // the height of a line of text
	gap    float64       // the spacing unit
	indent float64       // how far each tree level steps in
	names  decker.Rect   // the left column; names are shortened to fit it
	plot   decker.Rect   // where the bars go
	rows   []decker.Rect // one per span, across the names and the plot
	axis   decker.Rect   // the tick labels, under the plot
	legend decker.Rect   // the services along the bottom; empty if squeezed out
	ms     decker.Scale  // milliseconds onto the plot's x
	labels []durLabel    // one per span

	needW, needH float64 // the room the whole trace needs, unsqueezed
}

// durLabel is where a span's duration label goes.
type durLabel struct {
	text   string
	x      float64 // its left edge
	inside bool    // on the bar, so drawn in the background color
	ok     bool    // false when it fits nowhere, and is left out
}

// layoutWaterfall lays spans out in r; squeeze trades the legend, the tail
// spans and the ends of long names for room. It only measures and draws
// nothing, so it is cheap to call twice.
func layoutWaterfall(c decker.Ctx, r decker.Rect, spans []span, squeeze bool) waterfallLayout {
	th := c.Theme
	l := waterfallLayout{gap: c.Unit(0.02)}
	l.text = decker.Text{Font: th.Body, Size: c.SmallText(th.Body)}
	_, l.lineH = l.text.Measure("Ag")
	l.indent = float64(l.text.Size)
	right := r.Right()

	// The axis runs to where the trace ends; the plot needs room for a few
	// of its tick labels.
	end := 0.0
	for _, s := range spans {
		end = max(end, s.start+s.dur)
	}
	probe := decker.NiceScale(0, end, 5, 0, 1)
	tickW, _ := l.text.Measure(probe.Label(probe.Max) + "ms")
	minPlot := 4 * tickW

	// Rows need room for a name with a little air above and below.
	rowH := l.lineH * 1.12
	axisH := l.lineH + l.gap/3
	legendH := l.lineH + l.gap/2
	nameW := l.namesWidth(spans)
	l.needW = max(nameW+l.gap+minPlot, legendWidth(c, l.text))
	l.needH = float64(len(spans))*rowH + axisH + legendH
	if squeeze {
		legendH = 0
		spans = spans[:min(max(int((r.H-axisH)/rowH), 0), len(spans))]
		nameW = min(l.namesWidth(spans), r.W-l.gap-minPlot)
	}
	l.spans = spans

	// Cut the rect: legend and axis off the bottom, the names off the left,
	// half a tick label off the right so the last label stays on the page.
	if legendH > 0 {
		l.legend, r = r.CutBottom(legendH)
		_, l.legend = l.legend.CutTop(l.gap / 2)
	}
	l.axis, r = r.CutBottom(axisH)
	l.names, l.plot = r.CutLeft(nameW + l.gap)
	_, l.plot = l.plot.CutRight(tickW / 2)
	ones := make([]float64, len(spans))
	for i := range ones {
		ones[i] = 1
	}
	l.rows = r.Rows(0, ones...)

	// The finest round step that leaves room between tick labels. The axis
	// then keeps the trace's own end rather than NiceScale's rounded one,
	// so the root span runs the width of the plot.
	for n := 10; n >= 2; n-- {
		l.ms = decker.NiceScale(0, end, n, l.plot.X, l.plot.Right())
		if end > 0 {
			l.ms.Max = end
		}
		if l.ms.At(l.ms.Step)-l.ms.At(0) >= tickW*1.3 {
			break
		}
	}

	// Duration labels go inside the bar when they fit there with padding,
	// else after it, else before it. They're placed from the bar's final
	// width, so a label doesn't jump while its bar grows.
	pad := float64(l.text.Size) * 0.4
	l.labels = make([]durLabel, len(spans))
	for i, s := range spans {
		d := durLabel{text: strconv.FormatFloat(s.dur, 'f', -1, 64) + "ms", ok: true}
		w, _ := l.text.Measure(d.text)
		x0, x1 := l.ms.At(s.start), l.ms.At(s.start+s.dur)
		switch {
		case w+2*pad <= x1-x0:
			d.x, d.inside = x0+pad, true
		case x1+pad+w <= right:
			d.x = x1 + pad
		case x0-pad-w >= l.plot.X:
			d.x = x0 - pad - w
		default:
			d.ok = false
		}
		l.labels[i] = d
	}
	return l
}

// namesWidth is the width of the widest name, indented to its depth.
func (l waterfallLayout) namesWidth(spans []span) float64 {
	w := 0.0
	for i, s := range spans {
		sw, _ := l.text.Measure(s.name)
		w = max(w, float64(depth(spans, i))*l.indent+sw)
	}
	return w
}

// drawWaterfall draws l, revealing span i on build i and the critical path
// on build hilite.
func drawWaterfall(c decker.Ctx, p *decker.Pixels, l waterfallLayout, hilite int) {
	th := c.Theme
	focus := decker.Ease(c.Since(hilite), 0.5)
	crit := criticalPath(l.spans)

	// Gridlines and tick labels.
	tick := l.text
	tick.Color, tick.Align = th.Muted, decker.Center
	for v := range l.ms.Ticks() {
		x := l.ms.At(v)
		p.Rect(x, l.plot.Y, 1, l.plot.H, th.Faint, 1)
		tick.Draw(p, l.ms.Label(v)+"ms", x, l.axis.Y+l.gap/3)
	}

	for i, s := range l.spans {
		if !c.Reached(i) {
			continue
		}
		t := c.Since(i)
		row := l.rows[i]
		cy := row.Y + row.H/2
		d := float64(depth(l.spans, i))
		onPath := crit[i]

		// Off the critical path, everything fades back on the last build.
		dim := func(col decker.RGB) decker.RGB {
			if onPath {
				return col
			}
			return decker.Mix(col, th.Background, 0.65*focus)
		}

		// The tree guide: an elbow from the parent's row down to the name.
		if s.parent >= 0 {
			pr := l.rows[s.parent]
			gx := l.names.X + (d-1)*l.indent + l.indent*0.35
			grow := decker.Ease(t, 0.3)
			y0 := pr.Y + pr.H/2 + l.lineH*0.45
			w := max(c.Unit(0.005), 1)
			p.Line(gx, y0, gx, y0+(cy-y0)*grow, w, dim(th.Faint), 1)
			if grow >= 1 {
				p.Line(gx, cy, l.names.X+d*l.indent-l.indent*0.25, cy, w, dim(th.Faint), 1)
			}
		}

		// The name, at its depth, shortened if the column was squeezed.
		// Draw puts every line on the same baseline in its line box, where
		// DrawMid would center each name's ink and names with and without
		// descenders would sit unevenly.
		top := cy - l.lineH/2
		name := l.text
		name.Color = dim(decker.Mix(th.Text, th.Warn, focus*b2f(onPath)))
		name.FX = decker.FadeUp(t, 0.3, name.Size)
		x := l.names.X + d*l.indent
		name.Draw(p, elide(name, s.name, l.names.Right()-l.gap-x), x, top)

		// The bar grows from its start to its end.
		barH := row.H * barFrac
		x0, x1 := l.ms.At(s.start), l.ms.At(s.start+s.dur)
		grown := x0 + (x1-x0)*decker.EaseInOutCubic(decker.Progress(t, 0.1, 0.6))
		rad := min(barH/4, l.gap/2)
		p.RoundRect(x0, cy-barH/2, max(grown-x0, 1), barH, rad, 0, dim(th.SeriesColor(serviceIndex(s.service))), 1)
		if onPath && focus > 0 {
			p.RoundRect(x0-1, cy-barH/2-1, x1-x0+2, barH+2, rad+1, max(c.Unit(0.006), 1), th.Warn, focus)
		}

		// The duration label, once the bar has grown.
		lab := l.labels[i]
		if !lab.ok {
			c.Report(decker.SeverityWarning, "label-dropped", row, "waterfall: no room for the duration of "+s.name)
			continue
		}
		dur := l.text
		dur.Color = dim(th.Muted)
		if lab.inside {
			dur.Color = dim(th.Background)
		}
		dur.FX = decker.FadeUp(t-0.6, 0.3, dur.Size)
		dur.Draw(p, lab.text, lab.x, top)
	}

	if !l.legend.Empty() {
		drawLegend(c, p, l)
	}
}

// drawLegend writes the services along the bottom, each after a swatch of
// its color, on one baseline.
func drawLegend(c decker.Ctx, p *decker.Pixels, l waterfallLayout) {
	th := c.Theme
	txt := l.text
	txt.Color = th.Muted
	txt.FX = decker.FadeUp(c.T-0.3, 0.4, txt.Size)
	sw := l.lineH * 0.55
	x, cy := l.legend.X, l.legend.Y+l.lineH/2
	a := decker.Ease(c.T-0.3, 0.4)
	for i, s := range services {
		p.RoundRect(x, cy-sw/2, sw, sw, sw/4, 0, th.SeriesColor(i), a)
		x += sw + l.gap/2
		w, _ := txt.Draw(p, s, x, l.legend.Y)
		x += w + l.gap*1.5
	}
}

// legendWidth is the width drawLegend needs.
func legendWidth(c decker.Ctx, txt decker.Text) float64 {
	_, lineH := txt.Measure("Ag")
	gap := c.Unit(0.02)
	w := 0.0
	for _, s := range services {
		sw, _ := txt.Measure(s)
		w += lineH*0.55 + gap/2 + sw + gap*1.5
	}
	return w - gap*1.5
}

// elide returns s, or as much of it as fits in w followed by "…".
func elide(txt decker.Text, s string, w float64) string {
	if sw, _ := txt.Measure(s); sw <= w {
		return s
	}
	for r := []rune(s); len(r) > 1; {
		r = r[:len(r)-1]
		short := string(r) + "…"
		if sw, _ := txt.Measure(short); sw <= w {
			return short
		}
	}
	return ""
}

// depth counts how many parents span i has.
func depth(spans []span, i int) int {
	d := 0
	for p := spans[i].parent; p >= 0 && d < len(spans); p = spans[p].parent {
		d++
	}
	return d
}

// criticalPath marks the spans that decided when the trace ended: from the
// root, each step goes to the child that finished last.
func criticalPath(spans []span) []bool {
	on := make([]bool, len(spans))
	at := -1
	for i, s := range spans {
		if s.parent == -1 {
			at = i
			break
		}
	}
	for n := 0; at >= 0 && n < len(spans); n++ {
		on[at] = true
		next, end := -1, -1.0
		for i, s := range spans {
			if s.parent == at && s.start+s.dur > end {
				next, end = i, s.start+s.dur
			}
		}
		at = next
	}
	return on
}

// serviceIndex is the position of name in services, which picks its color.
func serviceIndex(name string) int {
	for i, s := range services {
		if s == name {
			return i
		}
	}
	return len(services)
}

func b2f(b bool) float64 {
	if b {
		return 1
	}
	return 0
}
