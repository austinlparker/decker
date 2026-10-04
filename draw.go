package decker

import (
	"math"
	"strconv"
	"strings"
)

// Panel draws a rounded box with a centered, auto-fitted label. A zero edge
// draws no outline; alpha fades the whole panel, label included.
//
// Panel is also the model for the stock components that follow and for new
// ones: take (c Ctx, p *Pixels, ...) with pixel coordinates, size things from
// c.Unit and c.SmallText, take colors and fonts from c.Theme, reveal builds
// with c.Reached and c.Since, and return the size drawn when callers lay out
// around it. A component with many options is a struct with a Draw method, as
// CycleDiagram is.
func Panel(c Ctx, p *Pixels, x, y, w, h float64, label string, fill, edge, text RGB, alpha float64) {
	r := min(c.Unit(0.03), w/3, h/3)
	p.RoundRect(x, y, w, h, r, 0, fill, alpha)
	if edge != (RGB{}) {
		p.RoundRect(x, y, w, h, r, max(c.Unit(0.008), 1), edge, alpha)
	}
	if label != "" {
		s, t := c.Theme.Body.Fit(label, w-c.Unit(0.04), h-c.Unit(0.02), c.Size(0.1), 0)
		tx := Text{Font: c.Theme.Body, Size: s, Align: Center, Color: Mix(fill, text, alpha)}
		if lines := float64(strings.Count(t, "\n") + 1); lines > 1 {
			tx.Draw(p, t, x+w/2, y+h/2-lines*float64(s)*DefaultLeading/2)
		} else {
			tx.DrawMid(p, t, x+w/2, y+h/2)
		}
	}
}

// Arrow strokes a line from (x0, y0) to (x1, y1) with an arrowhead at its tip,
// revealed up to fraction prog (0..1) of its length.
func Arrow(p *Pixels, x0, y0, x1, y1, width float64, col RGB, prog float64) {
	if prog <= 0 {
		return
	}
	prog = min(prog, 1)
	ex, ey := Lerp(x0, x1, prog), Lerp(y0, y1, prog)
	p.Line(x0, y0, ex, ey, width, col, 1)
	ang := math.Atan2(y1-y0, x1-x0)
	head := width * 3.2
	for _, s := range []float64{-1, 1} {
		a := ang + math.Pi - s*0.5
		p.Line(ex, ey, ex+head*math.Cos(a), ey+head*math.Sin(a), width, col, 1)
	}
}

// Label draws small body-font text at (x, y) and returns its size.
func Label(c Ctx, p *Pixels, s string, x, y float64, col RGB, align Align) (float64, float64) {
	size := c.SmallText(c.Theme.Body)
	return Text{Font: c.Theme.Body, Size: size, Color: col, Align: align}.Draw(p, s, x, y)
}

// LineLabel draws small text centered on (cx, cy) over a background plate,
// so it can sit on top of an arrow or line and stay readable.
func LineLabel(c Ctx, p *Pixels, f *Font, s string, cx, cy float64, col RGB, alpha float64) {
	size := c.SmallText(f)
	w := f.Measure(s, size)
	pad := float64(size) * 0.3
	p.Rect(cx-w/2-pad, cy-float64(size)*0.55, w+2*pad, float64(size)*1.1, c.Theme.Background, alpha)
	plateText(p, f, size, s, cx, cy, col, c.Theme.Background, alpha)
}

// Chip draws a small rounded tag with text, its ink centered in the tag;
// returns its width.
func Chip(c Ctx, p *Pixels, s string, x, y float64, fg, bg RGB, alpha float64) float64 {
	size := c.SmallText(c.Theme.Body)
	w := c.Theme.Body.Measure(s, size) + float64(size)*0.9
	h := ChipHeight(c)
	p.RoundRect(x, y, w, h, h/2, 0, bg, alpha)
	plateText(p, c.Theme.Body, size, s, x+w/2, y+h/2, fg, bg, alpha)
	return w
}

// ChipHeight is the height of a Chip; use it to lay out rows of chips.
func ChipHeight(c Ctx) float64 { return float64(c.SmallText(c.Theme.Body)) * 1.3 }

// plateText draws s centered on (cx, cy) in fg faded by alpha toward the plate
// color bg it sits on.
func plateText(p *Pixels, f *Font, size int, s string, cx, cy float64, fg, bg RGB, alpha float64) {
	Text{Font: f, Size: size, Align: Center, Color: Mix(bg, fg, alpha)}.DrawMid(p, s, cx, cy)
}

// wrappedHeight is the height of s wrapped to maxW at the default leading.
func wrappedHeight(f *Font, s string, size int, maxW float64) float64 {
	return float64(len(f.Wrap(s, size, maxW))) * float64(size) * DefaultLeading
}

// CycleDiagram is a ring of numbered stations with a legend. Stations
// appear one per step; once the ring closes, a comet circles it and lights
// up each station (and its legend line) as it passes.
type CycleDiagram struct {
	Labels []string
	Lap    float64 // seconds per loop; must be > 0
	Step0  int     // step at which the first station appears
	Ring   RGB     // the ring's track; zero uses the theme's Faint
}

// cycleGeom is where a CycleDiagram's ring and legend sit.
type cycleGeom struct {
	n                int
	cx, cy, R, nodeR float64
	ring             float64 // track thickness
	legendX, legendW float64
}

func (g cycleGeom) angle(i int) float64 { return float64(i) * 2 * math.Pi / float64(g.n) }

func (g cycleGeom) at(a float64) (float64, float64) {
	return g.cx + g.R*math.Sin(a), g.cy - g.R*math.Cos(a)
}

// Draw renders the diagram below y=top. It returns whether the comet is
// circling, its angle (0 = top, clockwise), the active station (-1 if none),
// and the x where the legend starts.
func (d CycleDiagram) Draw(c Ctx, p *Pixels, top float64) (looping bool, theta float64, active int, legendX float64) {
	g := d.geometry(c, top)
	last := d.Step0 + g.n - 1
	d.drawRing(c, p, g)

	active = -1
	looping = c.Reached(last) && c.Since(last) > 0.7
	if looping {
		theta, active = d.drawComet(c, p, g, c.Since(last)-0.7)
	}

	d.drawStations(c, p, g, looping, active)
	d.drawLegend(c, p, g, top, looping, active)
	return looping, theta, active, g.legendX
}

func (d CycleDiagram) geometry(c Ctx, top float64) cycleGeom {
	cy := (top + c.Y(0.88)) / 2
	R := min((c.Y(0.88)-top)*0.36, c.X(0.16))
	nodeR := R * 0.27
	// Center the ring and legend together, not their individual anchors.
	// Reserve enough legend width for longer labels to wrap cleanly.
	legendW := c.X(0.45)
	gap := c.Unit(0.06)
	groupW := 2*(R+nodeR) + gap + legendW
	groupX := (c.X(1) - groupW) / 2
	return cycleGeom{
		n: len(d.Labels), cx: groupX + R + nodeR, cy: cy, R: R, nodeR: nodeR,
		ring:    max(c.Unit(0.01), 1.5),
		legendX: groupX + 2*(R+nodeR) + gap, legendW: legendW,
	}
}

// drawRing draws the ring segments themselves as stations appear; the
// closing segment arrives with the last station.
func (d CycleDiagram) drawRing(c Ctx, p *Pixels, g cycleGeom) {
	track := d.Ring
	if track == (RGB{}) {
		track = c.Theme.Faint
	}
	last := d.Step0 + g.n - 1
	for i := 0; i < g.n; i++ {
		reveal := min(d.Step0+i+1, last)
		if !c.Reached(reveal) {
			continue
		}
		sweep := Ease(c.Since(reveal)-0.1, 0.5) * 2 * math.Pi / float64(g.n)
		p.Arc(g.cx, g.cy, g.R, g.ring, g.angle(i), g.angle(i)+sweep, track, 1)
	}
}

// drawComet draws the comet after lapT seconds of circling and returns its
// angle and the station it is passing (-1 if none).
func (d CycleDiagram) drawComet(c Ctx, p *Pixels, g cycleGeom, lapT float64) (theta float64, active int) {
	theta = math.Mod(lapT/d.Lap*2*math.Pi, 2*math.Pi)
	for k := 28; k >= 0; k-- {
		x, y := g.at(theta - float64(k)*0.035)
		f := 1 - float64(k)/29
		p.Disc(x, y, g.ring*0.5+g.ring*1.6*f, c.Theme.Accent, f*f)
	}
	hx, hy := g.at(theta)
	p.Glow(hx, hy, g.nodeR*1.4, c.Theme.Accent, 0.6)
	active = -1
	for i := 0; i < g.n; i++ {
		if math.Abs(math.Remainder(theta-g.angle(i), 2*math.Pi)) < 0.28 {
			active = i
		}
	}
	return theta, active
}

func (d CycleDiagram) drawStations(c Ctx, p *Pixels, g cycleGeom, looping bool, active int) {
	for i := 0; i < g.n; i++ {
		step := d.Step0 + i
		if !c.Reached(step) {
			continue
		}
		x, y := g.at(g.angle(i))
		pop := EaseOutBack(Progress(c.Since(step), 0, 0.4))
		edge := Mix(c.Theme.Accent, c.Theme.Muted, Progress(c.Since(step), 0.3, 0.8))
		if looping {
			edge = c.Theme.Faint
		}
		nc := c.Theme.Text
		if i == active {
			edge, nc = c.Theme.Accent, c.Theme.Accent
			p.Glow(x, y, g.nodeR*2.2, c.Theme.Accent, 0.35)
		}
		p.Disc(x, y, g.nodeR*pop, c.Theme.Panel, 1)
		p.Arc(x, y, g.nodeR*pop, max(c.Unit(0.009), 1.5), 0, 2*math.Pi, edge, 1)
		ns := c.Theme.Display.Drawn(int(g.nodeR * 1.15))
		Text{Font: c.Theme.Display, Size: ns, Align: Center, Color: nc, FX: FadeUp(c.Since(step)-0.1, 0.25, ns)}.
			Draw(p, strconv.Itoa(i+1), x, y-float64(ns)*0.6)
	}
}

// drawLegend draws one numbered line per station at a single text size: the
// biggest at which all wrapped labels fit in the room below top. Each label
// starts below the last.
func (d CycleDiagram) drawLegend(c Ctx, p *Pixels, g cycleGeom, top float64, looping bool, active int) {
	room := c.Y(0.88) - top
	widest := strings.Repeat("0", len(strconv.Itoa(g.n)))
	numW := func(s int) float64 { return c.Theme.Display.Measure(widest, s) + c.Unit(0.03) }
	height := func(s int) float64 {
		h := 0.0
		for _, l := range d.Labels {
			h += wrappedHeight(c.Theme.Body, l, s, g.legendW-numW(s))
		}
		return h + float64(g.n-1)*float64(s)*0.25
	}
	ls := c.Theme.Body.Drawn(c.Size(0.08))
	for ls > c.SmallText(c.Theme.Body) && height(ls) > room {
		ls = c.Theme.Body.Drawn(ls - 1)
	}
	labelW := g.legendW - numW(ls)
	y := top + max(0, (room-height(ls))/2)
	for i, l := range d.Labels {
		step := d.Step0 + i
		if !c.Reached(step) {
			continue
		}
		col, num := c.Theme.Text, c.Theme.Accent
		if looping && i != active {
			col, num = c.Theme.Muted, Mix(c.Theme.Accent, c.Theme.Background, 0.5)
		}
		Text{Font: c.Theme.Display, Size: ls, Color: num, FX: FadeUp(c.Since(step), 0.3, ls)}.Draw(p, strconv.Itoa(i+1), g.legendX, y)
		Text{Font: c.Theme.Body, Size: ls, Color: col, MaxW: labelW, FX: RiseIn(c.Since(step)-0.05, 0.012, ls)}.
			Draw(p, l, g.legendX+numW(ls), y)
		y += wrappedHeight(c.Theme.Body, l, ls, labelW) + float64(ls)*0.25
	}
}

// BulletList draws lines as bullets, revealing line i at step i+firstStep.
// It picks one text size for all lines that fits the box.
func BulletList(c Ctx, p *Pixels, lines []string, x, y, w, h float64, firstStep int) {
	// The mark is 0.32 of the text size wide; keep a gap after it that
	// grows with the text, so big bullets don't crowd their words.
	indent := func(size int) float64 { return max(c.Unit(0.05), float64(size)*0.6) }
	size := c.Size(0.17)
	for {
		total := 0.0
		for _, l := range lines {
			total += wrappedHeight(c.Theme.Body, l, size, w-indent(size))
		}
		total += float64(len(lines)-1) * float64(size) * 0.45
		if total <= h || size <= c.SmallText(c.Theme.Body) {
			break
		}
		size--
	}
	size = c.Theme.Body.Drawn(size)
	for i, l := range lines {
		step := i + firstStep
		if !c.Reached(step) {
			break
		}
		since := c.Since(step)
		col := c.Theme.Text
		if c.Step > step {
			col = c.Theme.Muted
		}
		r := float64(size) * 0.2 * EaseOutBack(Progress(since, 0, 0.3))
		p.Rect(x, y+float64(size)*0.3, r*1.6, float64(size)*0.55, c.Theme.Accent, 1)
		_, lh := Text{Font: c.Theme.Body, Size: size, Color: col, MaxW: w - indent(size),
			FX: RiseIn(since-0.05, 0.01, size)}.Draw(p, l, x+indent(size), y)
		y += lh + float64(size)*0.45
	}
}

// SpeechBubble draws a rounded bubble with a tail toward (tailX, tailY).
func SpeechBubble(p *Pixels, x, y, w, h, tailX, tailY float64, fill, edge RGB, alpha float64) {
	r := min(w, h) * 0.25
	p.Line(x+w*0.18, y+h*0.8, tailX, tailY, max(h*0.12, 2), fill, alpha)
	p.RoundRect(x, y, w, h, r, 0, fill, alpha)
	p.RoundRect(x, y, w, h, r, max(h*0.03, 1), edge, alpha)
}

// PlaceholderBox marks an area where real material (a screenshot, a real
// graph) still needs to go: a dashed frame with a label.
func PlaceholderBox(c Ctx, p *Pixels, x, y, w, h float64, what string) {
	dash := c.Unit(0.03)
	col := Mix(c.Theme.Background, c.Theme.Warn, 0.8)
	edge := func(x0, y0, x1, y1 float64) {
		l := math.Hypot(x1-x0, y1-y0)
		for d := 0.0; d < l; d += 2 * dash {
			e := min(d+dash, l)
			p.Line(x0+(x1-x0)*d/l, y0+(y1-y0)*d/l, x0+(x1-x0)*e/l, y0+(y1-y0)*e/l, 1.5, col, 1)
		}
	}
	edge(x, y, x+w, y)
	edge(x+w, y, x+w, y+h)
	edge(x+w, y+h, x, y+h)
	edge(x, y+h, x, y)
	s := c.SmallText(c.Theme.Body)
	Text{Font: c.Theme.Body, Size: s, Align: Center, Color: c.Theme.Warn, MaxW: w - c.Unit(0.06)}.
		Draw(p, "PLACEHOLDER\n"+what, x+w/2, y+h/2-float64(s)*DefaultLeading)
}

// IllustrativeTag labels a chart whose data is made up for the sketch and
// must be replaced before the talk. (x, y) is the tag's top-right corner.
func IllustrativeTag(c Ctx, p *Pixels, x, y float64) {
	Label(c, p, "ILLUSTRATIVE DATA", x, y, c.Theme.Warn, Right)
}
