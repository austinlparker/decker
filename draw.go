package decker

import (
	"math"
	"strings"
)

// Stock components for diagram slides: boxes, arrows, labels, chips, a
// cycle diagram, bullet lists. They draw in the theme's colors and fonts
// (c.Theme), so they fit any deck. All coordinates are pixels.

// Panel draws a rounded box with a centered pixel-font label. edge may be
// zero for no outline.
func Panel(c Ctx, p *Pixels, x, y, w, h float64, label string, fill, edge, text RGB, alpha float64) {
	r := math.Min(c.Unit(0.03), math.Min(w, h)/3)
	p.RoundRect(x, y, w, h, r, 0, fill, alpha)
	if edge != (RGB{}) {
		p.RoundRect(x, y, w, h, r, math.Max(c.Unit(0.008), 1), edge, alpha)
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

// Arrow draws a line from (x0,y0) toward (x1,y1) with an arrowhead,
// revealed up to fraction prog (0..1) of its length.
func Arrow(p *Pixels, x0, y0, x1, y1, width float64, col RGB, prog float64) {
	if prog <= 0 {
		return
	}
	prog = math.Min(prog, 1)
	ex, ey := Lerp(x0, x1, prog), Lerp(y0, y1, prog)
	p.Line(x0, y0, ex, ey, width, col, 1)
	ang := math.Atan2(y1-y0, x1-x0)
	head := width * 3.2
	for _, s := range []float64{-1, 1} {
		a := ang + math.Pi - s*0.5
		p.Line(ex, ey, ex+head*math.Cos(a), ey+head*math.Sin(a), width, col, 1)
	}
}

// Label draws small pixel text; returns its size.
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
	Text{Font: f, Size: size, Align: Center, Color: Mix(c.Theme.Background, col, alpha)}.DrawMid(p, s, cx, cy)
}

// Chip draws a small rounded tag with text, its ink centered in the tag;
// returns its width.
func Chip(c Ctx, p *Pixels, s string, x, y float64, fg, bg RGB, alpha float64) float64 {
	size := c.SmallText(c.Theme.Body)
	w := c.Theme.Body.Measure(s, size) + float64(size)*0.9
	h := ChipHeight(c)
	p.RoundRect(x, y, w, h, h/2, 0, bg, alpha)
	Text{Font: c.Theme.Body, Size: size, Align: Center, Color: Mix(bg, fg, alpha)}.DrawMid(p, s, x+w/2, y+h/2)
	return w
}

// ChipHeight is the height of a Chip.
func ChipHeight(c Ctx) float64 { return float64(c.SmallText(c.Theme.Body)) * 1.3 }

// CycleDiagram is a ring of numbered stations with a legend. Stations
// appear one per step; once the ring closes, a comet circles it and lights
// up each station (and its legend line) as it passes.
type CycleDiagram struct {
	Labels []string
	Lap    float64 // seconds per loop
	Step0  int     // step at which the first station appears
	Ring   RGB     // the ring's track; zero uses the theme's Faint
}

// Draw renders the diagram below y=top. It returns whether the comet is
// circling, its angle (0 = top, clockwise), the active station (-1 if none),
// and the x where the legend starts.
func (d CycleDiagram) Draw(c Ctx, p *Pixels, top float64) (looping bool, theta float64, active int, legendX float64) {
	n := len(d.Labels)
	last := d.Step0 + n - 1
	cy := (top + c.Y(0.88)) / 2
	R := math.Min((c.Y(0.88)-top)*0.36, c.X(0.16))
	nodeR := R * 0.27
	// Center the ring and legend together, not their individual anchors.
	// Reserve enough legend width for longer labels to wrap cleanly.
	legendW := c.X(0.45)
	gap := c.Unit(0.06)
	groupW := 2*(R+nodeR) + gap + legendW
	groupX := (c.X(1) - groupW) / 2
	cx := groupX + R + nodeR
	legendX = groupX + 2*(R+nodeR) + gap
	ring := math.Max(c.Unit(0.01), 1.5)
	track := d.Ring
	if track == (RGB{}) {
		track = c.Theme.Faint
	}
	angle := func(i int) float64 { return float64(i) * 2 * math.Pi / float64(n) }
	at := func(a float64) (float64, float64) { return cx + R*math.Sin(a), cy - R*math.Cos(a) }

	// Ring segments draw themselves as stations appear; the closing segment
	// arrives with the last station.
	for i := 0; i < n; i++ {
		reveal := min(d.Step0+i+1, last)
		if !c.Reached(reveal) {
			continue
		}
		sweep := Ease(c.Since(reveal)-0.1, 0.5) * 2 * math.Pi / float64(n)
		p.Arc(cx, cy, R, ring, angle(i), angle(i)+sweep, track, 1)
	}

	active = -1
	looping = c.Reached(last) && c.Since(last) > 0.7
	if looping {
		theta = math.Mod((c.Since(last)-0.7)/d.Lap*2*math.Pi, 2*math.Pi)
		for k := 28; k >= 0; k-- {
			x, y := at(theta - float64(k)*0.035)
			f := 1 - float64(k)/29
			p.Disc(x, y, ring*0.5+ring*1.6*f, c.Theme.Accent, f*f)
		}
		hx, hy := at(theta)
		p.Glow(hx, hy, nodeR*1.4, c.Theme.Accent, 0.6)
		for i := 0; i < n; i++ {
			if math.Abs(math.Remainder(theta-angle(i), 2*math.Pi)) < 0.28 {
				active = i
			}
		}
	}

	for i := 0; i < n; i++ {
		step := d.Step0 + i
		if !c.Reached(step) {
			continue
		}
		x, y := at(angle(i))
		pop := EaseOutBack(Progress(c.Since(step), 0, 0.4))
		edge := Mix(c.Theme.Accent, c.Theme.Muted, Progress(c.Since(step), 0.3, 0.8))
		if looping {
			edge = c.Theme.Faint
		}
		nc := c.Theme.Text
		if i == active {
			edge, nc = c.Theme.Accent, c.Theme.Accent
			p.Glow(x, y, nodeR*2.2, c.Theme.Accent, 0.35)
		}
		p.Disc(x, y, nodeR*pop, c.Theme.Panel, 1)
		p.Arc(x, y, nodeR*pop, math.Max(c.Unit(0.009), 1.5), 0, 2*math.Pi, edge, 1)
		ns := c.Theme.Display.Drawn(int(nodeR * 1.15))
		Text{Font: c.Theme.Display, Size: ns, Align: Center, Color: nc, FX: FadeUp(c.Since(step)-0.1, 0.25, ns)}.
			Draw(p, string(rune('1'+i)), x, y-float64(ns)*0.6)
	}

	// One size for every line: the biggest at which all wrapped labels
	// fit in the room below the title. Each label starts below the last.
	room := c.Y(0.88) - top
	numW := func(s int) float64 { return c.Theme.Display.Measure("0", s) + c.Unit(0.03) }
	height := func(s int) float64 {
		h := 0.0
		for _, l := range d.Labels {
			h += float64(len(c.Theme.Body.Wrap(l, s, legendW-numW(s)))) * float64(s) * DefaultLeading
		}
		return h + float64(n-1)*float64(s)*0.25
	}
	ls := c.Theme.Body.Drawn(c.Size(0.08))
	for ls > c.SmallText(c.Theme.Body) && height(ls) > room {
		ls = c.Theme.Body.Drawn(ls - 1)
	}
	labelW := legendW - numW(ls)
	y := top + math.Max(0, (room-height(ls))/2)
	for i, l := range d.Labels {
		step := d.Step0 + i
		if !c.Reached(step) {
			continue
		}
		col, num := c.Theme.Text, c.Theme.Accent
		if looping && i != active {
			col, num = c.Theme.Muted, Mix(c.Theme.Accent, c.Theme.Background, 0.5)
		}
		Text{Font: c.Theme.Display, Size: ls, Color: num, FX: FadeUp(c.Since(step), 0.3, ls)}.Draw(p, string(rune('1'+i)), legendX, y)
		Text{Font: c.Theme.Body, Size: ls, Color: col, MaxW: labelW, FX: RiseIn(c.Since(step)-0.05, 0.012, ls)}.
			Draw(p, l, legendX+numW(ls), y)
		y += float64(len(c.Theme.Body.Wrap(l, ls, labelW)))*float64(ls)*DefaultLeading + float64(ls)*0.25
	}
	return looping, theta, active, legendX
}

// BulletList draws lines as bullets, revealing line i at step i+firstStep.
// It picks one text size for all lines that fits the box.
func BulletList(c Ctx, p *Pixels, lines []string, x, y, w, h float64, firstStep int) {
	// The mark is 0.32 of the text size wide; keep a gap after it that
	// grows with the text, so big bullets don't crowd their words.
	indent := func(size int) float64 { return math.Max(c.Unit(0.05), float64(size)*0.6) }
	size := c.Size(0.17)
	for {
		total := 0.0
		for _, l := range lines {
			total += float64(len(c.Theme.Body.Wrap(l, size, w-indent(size)))) * float64(size) * DefaultLeading
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
	r := math.Min(w, h) * 0.25
	p.Line(x+w*0.18, y+h*0.8, tailX, tailY, math.Max(h*0.12, 2), fill, alpha)
	p.RoundRect(x, y, w, h, r, 0, fill, alpha)
	p.RoundRect(x, y, w, h, r, math.Max(h*0.03, 1), edge, alpha)
}

// PlaceholderBox marks an area where real material (a screenshot, a real
// graph) still needs to go: a dashed frame with a label.
func PlaceholderBox(c Ctx, p *Pixels, x, y, w, h float64, what string) {
	dash := c.Unit(0.03)
	col := Mix(c.Theme.Background, c.Theme.Warn, 0.8)
	edge := func(x0, y0, x1, y1 float64) {
		l := math.Hypot(x1-x0, y1-y0)
		for d := 0.0; d < l; d += 2 * dash {
			e := math.Min(d+dash, l)
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
	s := c.SmallText(c.Theme.Body)
	Text{Font: c.Theme.Body, Size: s, Align: Right, Color: c.Theme.Warn}.Draw(p, "ILLUSTRATIVE DATA", x, y)
}
