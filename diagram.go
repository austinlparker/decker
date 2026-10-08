package decker

import "strings"

// TimelineItem is one milestone on a Timeline.
type TimelineItem struct {
	Label  string // the milestone's name, in the display font
	Detail string // optional supporting text beneath or beside it
}

// Timeline is a line with a dot per milestone, built one item per step: item i
// appears at step FirstStep+i, the line drawing on to its dot, the dot popping
// and the text fading up. The newest item is in Accent and earlier ones in
// Muted, as BulletList dims its lines. Text is fitted to its slot, all labels
// at one size and all details at another.
type Timeline struct {
	Items     []TimelineItem
	FirstStep int
	Vertical  bool // items run down the rect with text beside the line, not across it
}

// Draw renders the timeline in r, built out to the current step, and returns
// the size of the whole timeline once built: it fills the length of r along
// the line and takes as much across it as its text needs.
func (t Timeline) Draw(c Ctx, p *Pixels, r Rect) (w, h float64) {
	defer c.within("Timeline", r)()
	n := len(t.Items)
	if n == 0 {
		return 0, 0
	}
	th := c.Theme
	dotR := max(c.Unit(0.016), 3)
	lineW := max(c.Unit(0.006), 1.5)
	gap := c.Unit(0.02)

	// Horizontal items sit in equal columns with their text under the dot;
	// vertical ones in equal rows with text to its right.
	slotW, slotH := r.W/float64(n), r.H/float64(n)
	textX, textW, textH := 0.0, slotW-gap, r.H-dotR*2-gap
	if t.Vertical {
		textX = r.X + dotR*2 + gap
		textW, textH = r.W-(textX-r.X), slotH-gap/2
	}
	ls, ds, labelH, detailH, widest := t.fit(c, textW, textH)

	dot := func(i int) (x, y float64) {
		if t.Vertical {
			return r.X + dotR, r.Y + float64(i)*slotH + float64(ls)*0.55
		}
		return r.X + (float64(i)+0.5)*slotW, r.Y + dotR
	}
	for i, it := range t.Items {
		step := t.FirstStep + i
		if !c.Reached(step) {
			break
		}
		since := c.Since(step)
		col, body := th.Accent, th.Text
		if c.Step > step {
			col, body = th.Muted, th.Muted
		}
		x, y := dot(i)
		delay := 0.0 // the dot waits for the line to reach it
		if i > 0 {
			px, py := dot(i - 1)
			// A zero-length line still draws its round cap, over the last dot.
			if e := Ease(since, 0.35); e > 0 {
				p.Line(px, py, Lerp(px, x, e), Lerp(py, y, e), lineW, col, 1)
			}
			delay = 0.3
		}
		p.Disc(x, y, dotR*EaseOutBack(Progress(since, delay, 0.35)), col, 1)

		align, tx, ty, mw := Center, x, y+dotR+gap/2, textW
		if t.Vertical {
			align, tx, ty, mw = Left, textX, y-float64(ls)*0.55, textW
		}
		fx := FadeUp(since-delay-0.05, 0.3, ls)
		Text{Font: th.Display, Size: ls, Color: col, Align: align, MaxW: mw, FX: fx}.Draw(p, it.Label, tx, ty)
		if it.Detail != "" {
			dy := ty + labelH + float64(ds)*0.2
			Text{Font: th.Body, Size: ds, Color: body, Align: align, MaxW: mw, FX: FadeUp(since-delay-0.15, 0.3, ds)}.Draw(p, it.Detail, tx, dy)
		}
	}
	if t.Vertical {
		w, h = textX-r.X+widest, float64(n-1)*slotH+float64(ls)*0.55+labelH+detailH
	} else {
		w, h = r.W, dotR*2+gap/2+labelH+float64(ds)*0.2+detailH
	}
	if c.review != nil {
		needW := w
		if !t.Vertical && widest > textW { // a word wider than its slot
			needW = float64(n) * (widest + gap)
		}
		c.Fits("Timeline", r, needW, h)
	}
	return w, h
}

// fit finds the label and detail sizes that make every item's text fit
// textW×textH, and the tallest label block, tallest detail block and widest
// text at those sizes. Label and detail share the height, three to two.
func (t Timeline) fit(c Ctx, textW, textH float64) (ls, ds int, labelH, detailH, widest float64) {
	th := c.Theme
	ls, ds = c.Size(0.1), c.Size(0.07)
	lh, dh := max(textH*0.6, 1), max(textH*0.4, 1)
	for _, it := range t.Items {
		s, _ := th.Display.Fit(it.Label, textW, lh, ls, 0)
		ls = min(ls, s)
		if it.Detail != "" {
			s, _ = th.Body.Fit(it.Detail, textW, dh, ds, 0)
			ds = min(ds, s)
		}
	}
	// Past this floor shrinking stops helping: a smaller line is unreadable,
	// and the text runs a little past its slot instead.
	ls = max(th.Display.Drawn(ls), c.SmallText(th.Display))
	ds = max(th.Body.Drawn(ds), c.SmallText(th.Body))
	for _, it := range t.Items {
		lines := th.Display.wrapped(it.Label, ls, textW)
		labelH = max(labelH, float64(len(lines))*float64(ls)*DefaultLeading)
		widest = max(widest, th.Display.widest(lines, ls))
		lines = th.Body.wrapped(it.Detail, ds, textW)
		if it.Detail != "" {
			detailH = max(detailH, float64(len(lines))*float64(ds)*DefaultLeading)
		}
		widest = max(widest, th.Body.widest(lines, ds))
	}
	return ls, ds, labelH, detailH, widest
}

// Process is a row of chevrons, one per step of a procedure, revealed one at a
// time: step i appears at step FirstStep+i, sliding in from the left. The
// newest is filled in Accent and earlier ones are outlined in Muted. Text is
// fitted inside each shape, all at one size.
type Process struct {
	Steps     []string
	FirstStep int
}

// Draw renders the row at the top of r, built out to the current step, and
// returns its size once built: the width of r and a chevron's height.
func (pr Process) Draw(c Ctx, p *Pixels, r Rect) (w, h float64) {
	defer c.within("Process", r)()
	n := len(pr.Steps)
	if n == 0 {
		return 0, 0
	}
	th := c.Theme
	h = min(r.H, c.Unit(0.2))
	tip := h * 0.3
	gap := c.Unit(0.012)
	cw := (r.W + float64(n-1)*(tip-gap)) / float64(n) // chevron width, so the row spans r
	pad := c.Unit(0.015)
	textW, textH := cw-2*tip-pad, h*0.8

	size := c.Size(0.1)
	for _, s := range pr.Steps {
		fs, _ := th.Body.Fit(s, textW, textH, size, 0)
		size = min(size, fs)
	}
	size = max(th.Body.Drawn(size), c.SmallText(th.Body))
	if c.review != nil {
		for i, s := range pr.Steps {
			lines := th.Body.wrapped(s, size, textW)
			x := r.X + float64(i)*(cw-tip+gap)
			c.Fits(quoteText("Process step", s), Rect{x + tip, r.Y + (h-textH)/2, textW, textH},
				th.Body.widest(lines, size), float64(len(lines))*float64(size)*DefaultLeading)
		}
	}

	for i, s := range pr.Steps {
		step := pr.FirstStep + i
		if !c.Reached(step) {
			break
		}
		e := Ease(c.Since(step), 0.4)
		x := r.X + float64(i)*(cw-tip+gap) - tip*(1-e)
		y := r.Y
		var buf [14]float64 // the six corners, and the first again to close the outline
		pts := append(buf[:0], x, y, x+cw-tip, y, x+cw, y+h/2, x+cw-tip, y+h, x, y+h)
		left := pad / 2
		if i > 0 { // notched on the left to take the previous point
			pts = append(pts, x+tip, y+h/2)
			left = tip + pad/2
		}
		fill, edge, text := th.Panel, th.Muted, th.Muted
		if c.Step == step {
			fill, edge, text = th.Accent, th.Accent, th.Background
		}
		p.Polygon(pts, fill, e)
		closed := append(pts, pts[0], pts[1])
		p.Polyline(closed, max(c.Unit(0.005), 1), edge, e)

		mid := x + left + (cw-tip-left)/2
		wrapped := strings.Join(th.Body.wrapped(s, size, textW), "\n")
		lines := float64(strings.Count(wrapped, "\n") + 1)
		// The text fades with the shape: at e == 0 nothing of the step shows.
		tx := Text{Font: th.Body, Size: size, Align: Center, Color: text, FX: func(int) GlyphFX { return GlyphFX{Alpha: e} }}
		if lines > 1 {
			tx.Draw(p, wrapped, mid, y+h/2-lines*float64(size)*DefaultLeading/2)
		} else {
			tx.DrawMid(p, wrapped, mid, y+h/2)
		}
	}
	return r.W, h
}
