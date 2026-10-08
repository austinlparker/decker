package decker

import (
	"fmt"
	"math"
	"slices"
	"strings"
)

// TableReveal is how a Table builds up over steps.
type TableReveal int

const (
	// TableRevealAll shows the whole table at Table.FirstStep.
	TableRevealAll TableReveal = iota
	// TableRevealRows shows the header, then row i at step FirstStep+i.
	TableRevealRows
	// TableRevealCols shows column j, header cell included, at step
	// FirstStep+j.
	TableRevealCols
)

// Table draws a grid of text: an optional header, rows of cells, columns
// sized by weight. Every cell is set at one text size, the largest that fits
// the rect, so the numbers and words on a slide read as one piece.
//
// Rows and cells may be ragged: a missing cell is empty, and the column
// count is that of the longest row. A cell that cannot fit even at the
// smallest readable size is ellipsized, never drawn outside its column, and
// rows that would run past the rect are left out.
type Table struct {
	Header []string   // optional column titles, in the Display font and the accent color
	Rows   [][]string // body cells, in the Body font

	Weights []float64 // relative column widths; missing entries are 1
	Align   []Align   // per column; missing entries are Left

	Zebra bool // tint every other body row
	Rules bool // thin lines between body rows

	// FirstStep is the step at which the table starts to appear; Reveal says
	// how the rest follows.
	FirstStep int
	Reveal    TableReveal

	// Highlight is the 1-based body row to emphasize with an accent bar while
	// the other rows dim; 0 emphasizes none. It takes effect at once, so to
	// animate a highlight over steps use Walk.
	Highlight int
	// Walk makes the highlight follow the build: step FirstStep+i emphasizes
	// row i+1, gliding down from the row before it, and the last row stays
	// emphasized once the steps run out. Walk overrides Highlight.
	Walk bool
}

// tableMaxText is the largest table text size, as a fraction of the canvas
// height; a bigger table would read as a list of headlines.
const tableMaxText = 0.085

// tableLayout is a fitted table, in coordinates relative to the rect's
// top-left corner. It is shared between frames and must not be modified.
type tableLayout struct {
	size       int
	header     bool
	colX, colW []float64
	rowY, rowH []float64    // the header first, if there is one; only rows that fit
	lines      [][][]string // [row][col] wrapped lines, in step with rowY
	padX, padY float64
	h          float64 // bottom of the last row

	// For review: how many rows the table has, header included (those past
	// rowY were left out), and the cells cut short, by row and column.
	rows int
	cut  [][2]int
}

type tableKey struct {
	content    uint64
	body, head *Font
	w, h, ph   float64
}

// tableLayouts remembers fits: a table is laid out from scratch only when its
// content, the rect, the canvas or the fonts change, not every frame.
var tableLayouts = memo[tableKey, *tableLayout]{max: 256}

// contentHash is FNV-1a over everything that decides layout, so the memo key
// costs no allocation.
func (t Table) contentHash() uint64 {
	h := uint64(14695981039346656037)
	str := func(s string) {
		for i := 0; i < len(s); i++ {
			h = (h ^ uint64(s[i])) * 1099511628211
		}
		h = (h ^ 0xff) * 1099511628211
	}
	num := func(n uint64) { h = (h ^ n) * 1099511628211 }
	num(uint64(len(t.Header)))
	for _, s := range t.Header {
		str(s)
	}
	num(uint64(len(t.Rows)))
	for _, row := range t.Rows {
		num(uint64(len(row)))
		for _, s := range row {
			str(s)
		}
	}
	for _, w := range t.Weights {
		num(math.Float64bits(w))
	}
	return h
}

func (t Table) columns() int {
	n := len(t.Header)
	for _, row := range t.Rows {
		n = max(n, len(row))
	}
	return n
}

func (t Table) layout(c Ctx, r Rect) *tableLayout {
	k := tableKey{t.contentHash(), c.Theme.Body, c.Theme.Display, r.W, r.H, c.PH()}
	return tableLayouts.get(k, func() *tableLayout { return t.fit(c, r) })
}

// cell returns the text of body row i (or the header, for i < 0) at column j.
func (t Table) cell(i, j int) string {
	row := t.Header
	if i >= 0 {
		row = t.Rows[i]
	}
	if j < len(row) {
		return row[j]
	}
	return ""
}

// fit picks the largest size at which every row's wrapped cells stack into
// r.H, then lays the table out at it. If none does, it takes the smallest
// readable size and ellipsizes cells line by line until the rows fit, and past
// that drops the rows that still run over.
func (t Table) fit(c Ctx, r Rect) *tableLayout {
	ncols := t.columns()
	l := &tableLayout{header: len(t.Header) > 0, padX: c.Unit(0.02), padY: c.Unit(0.01)}
	if ncols == 0 {
		return l
	}
	weights := make([]float64, ncols)
	for j := range weights {
		weights[j] = 1
		if j < len(t.Weights) {
			weights[j] = t.Weights[j]
		}
	}
	for _, col := range r.Cols(0, weights...) {
		l.colX = append(l.colX, col.X-r.X)
		l.colW = append(l.colW, col.W)
	}
	inner := func(j int) float64 { return max(l.colW[j]-2*l.padX, 0) }

	first := 0 // index of the first row in the layout's row order
	if !l.header {
		first = 1
	}
	nrows := len(t.Rows) + 1 - first
	font := func(row int) *Font {
		if row == 0 && l.header {
			return c.Theme.Display
		}
		return c.Theme.Body
	}
	// wrap sets every cell at size and reports whether no word overflows its
	// column.
	wrap := func(size int) (lines [][][]string, fitsWidth bool) {
		fitsWidth = true
		lines = make([][][]string, nrows)
		for row := range lines {
			lines[row] = make([][]string, ncols)
			for j := range ncols {
				f := font(row)
				ls := f.Wrap(t.cell(row+first-1, j), size, inner(j))
				lines[row][j] = ls
				fitsWidth = fitsWidth && fits(ls, inner(j), func(s string) float64 { return f.Measure(s, size) })
			}
		}
		return lines, fitsWidth
	}
	rowLines := func(cells [][]string) int {
		n := 1
		for _, ls := range cells {
			n = max(n, len(ls))
		}
		return n
	}
	height := func(lines [][][]string, size, limit int) float64 {
		h := 0.0
		for _, cells := range lines {
			h += linesHeight(min(rowLines(cells), limit), size, DefaultLeading) + 2*l.padY
		}
		return h
	}

	minSize := c.SmallText(c.Theme.Body)
	var lines [][][]string
	size := largestSize(max(c.Theme.Body.Drawn(c.Size(tableMaxText)), minSize), minSize, func(size int) bool {
		var ok bool
		lines, ok = wrap(size)
		return ok && height(lines, size, math.MaxInt) <= r.H
	})

	limit := math.MaxInt
	if height(lines, size, limit) > r.H {
		for limit = 1; limit < 1<<20; limit++ {
			if height(lines, size, limit+1) > r.H {
				break
			}
		}
	}
	l.size = size
	y := 0.0
	for row, cells := range lines {
		rh := linesHeight(min(rowLines(cells), limit), size, DefaultLeading) + 2*l.padY
		if y+rh > r.H+1e-9 {
			break
		}
		for j := range cells {
			clipped := clipLines(font(row), cells[j], size, inner(j), limit)
			if !slices.Equal(clipped, cells[j]) {
				l.cut = append(l.cut, [2]int{row, j})
			}
			cells[j] = clipped
		}
		l.rowY, l.rowH = append(l.rowY, y), append(l.rowH, rh)
		l.lines = append(l.lines, cells)
		y += rh
	}
	l.h = y
	l.rows = len(lines)
	return l
}

// name is how a review message refers to the table.
func (t Table) name() string {
	if len(t.Header) > 0 {
		return quoteText("Table", t.Header[0])
	}
	return fmt.Sprintf("Table (%d rows)", len(t.Rows))
}

// report records the rows left out, header included, and the cells cut
// short among those this build reveals: a row or a column still to come is
// not lost yet.
func (t Table) report(c Ctx, r Rect, l *tableLayout) {
	off := 0 // layout rows before the first body row
	if l.header {
		off = 1
	}
	shown := func(row, col int) bool { return t.since(c, row-off, col) >= 0 }
	revealed, dropped := 0, 0
	for row := off; row < l.rows; row++ {
		if shown(row, 0) {
			revealed++
			if row >= len(l.rowY) {
				dropped++
			}
		}
	}
	cut := 0
	for _, rc := range l.cut {
		if shown(rc[0], rc[1]) {
			cut++
		}
	}
	name := t.name()
	// The header is laid out first, so it is lost only when nothing fits.
	var lost string
	switch header := l.header && shown(0, 0) && len(l.rowY) == 0; {
	case header && dropped > 0:
		lost = fmt.Sprintf("the header and %d of %d rows don't fit", dropped, revealed)
	case header:
		lost = "the header doesn't fit"
	case dropped > 0:
		lost = fmt.Sprintf("%d of %d rows don't fit", dropped, revealed)
	}
	if lost != "" {
		c.review.add(SeverityError, "table-rows-dropped", r,
			fmt.Sprintf("%s: %s in %.0fpx at %dpx text", name, lost, r.H, l.size))
	}
	if cut > 0 {
		c.review.add(SeverityWarning, "table-cell-cut", r,
			fmt.Sprintf("%s: %d cells cut short at %dpx text", name, cut, l.size))
	}
}

// clipLines keeps at most limit lines of ls and ellipsizes any that are wider
// than maxW, so a cell never leaves its column.
func clipLines(f *Font, ls []string, size int, maxW float64, limit int) []string {
	out := append([]string(nil), ls[:min(len(ls), limit)]...)
	if len(ls) > limit {
		out[limit-1] = ellipsize(f, strings.Join(ls[limit-1:], " "), size, maxW, true)
	}
	for i, s := range out {
		out[i] = ellipsize(f, s, size, maxW, false)
	}
	return out
}

// ellipsize trims s until it and an ellipsis fit maxW. With force the ellipsis
// is added even if s already fits, to show that text was cut after it.
func ellipsize(f *Font, s string, size int, maxW float64, force bool) string {
	const dots = "..."
	if !force && f.Measure(s, size) <= maxW {
		return s
	}
	rs := []rune(strings.TrimSpace(s))
	for len(rs) > 0 && f.Measure(string(rs)+dots, size) > maxW {
		rs = rs[:len(rs)-1]
	}
	if len(rs) == 0 && f.Measure(dots, size) > maxW {
		return ""
	}
	return strings.TrimSpace(string(rs)) + dots
}

// tableHighlight is the emphasized row at one frame: the row and where it
// came from (-1 for none), and how far along the move is.
type tableHighlight struct {
	to, from int
	e        float64
}

func (t Table) highlight(c Ctx, rows int) (h tableHighlight) {
	h = tableHighlight{to: -1, from: -1, e: 1}
	switch {
	case t.Walk && rows > 0 && c.Step >= t.FirstStep:
		h.to = min(c.Step-t.FirstStep, rows-1)
		if h.to > 0 {
			h.from = h.to - 1
		}
		h.e = EaseInOutCubic(Progress(c.Since(t.FirstStep+h.to), 0, 0.4))
	case !t.Walk && t.Highlight >= 1 && t.Highlight <= rows:
		h.to = t.Highlight - 1
	}
	return h
}

// since is how long ago body row i (header for i < 0) or column j began to
// appear; negative if it hasn't.
func (t Table) since(c Ctx, i, j int) float64 {
	switch t.Reveal {
	case TableRevealRows:
		if i >= 0 {
			return c.Since(t.FirstStep + i)
		}
	case TableRevealCols:
		return c.Since(t.FirstStep + j)
	}
	return c.Since(t.FirstStep)
}

// Draw renders the table inside r and returns the width and height it
// occupies, which is all of r's width and as much height as its rows need.
// The table is laid out the same every frame, so rows that haven't appeared
// yet still hold their place.
func (t Table) Draw(c Ctx, p *Pixels, r Rect) (w, h float64) {
	l := t.layout(c, r)
	if len(l.colW) == 0 {
		return 0, 0
	}
	if c.review != nil {
		t.report(c, r, l)
	}
	if !c.Reached(t.FirstStep) {
		return r.W, l.h
	}
	if c.review != nil {
		defer c.within(t.name(), r)()
	}
	th := c.Theme
	size := float64(l.size)
	rule := max(c.Unit(0.004), 1)
	nbody := len(l.rowY)
	if l.header {
		nbody--
	}
	hl := t.highlight(c, len(t.Rows))
	off := 0 // layout rows before the first body row
	if l.header {
		off = 1
	}

	// Backgrounds and rules come first so every cell's text lands on top.
	for row := range l.rowY {
		i := row - off
		y, rh := r.Y+l.rowY[row], l.rowH[row]
		a := Ease(t.since(c, i, 0), 0.3)
		if t.Reveal == TableRevealCols {
			a = Ease(c.Since(t.FirstStep), 0.3)
		}
		switch {
		case i < 0:
			p.Rect(r.X, y, r.W, rh, th.Panel, a)
			p.Rect(r.X, y+rh-rule, r.W, rule, th.Accent, a)
		case t.Zebra && i%2 == 1:
			p.Rect(r.X, y, r.W, rh, Mix(th.Background, th.Panel, 0.6), a)
		}
		if t.Rules && i >= 0 && i < nbody-1 {
			p.Rect(r.X, y+rh-rule/2, r.W, rule, th.Faint, a)
		}
	}
	if hl.to >= 0 && hl.to < nbody && c.Reached(t.rowStep(hl.to)) {
		t.drawHighlight(c, p, r, l, hl, off)
	}

	for row, cells := range l.lines {
		i := row - off
		for j, ls := range cells {
			s := t.since(c, i, j)
			if s < 0 || len(ls) == 0 {
				continue
			}
			f, col := th.Body, th.Text
			if i < 0 {
				f, col = th.Display, th.Accent
			} else if hl.to >= 0 {
				col = Mix(th.Text, th.Muted, 1-hl.weight(i))
			}
			var align Align
			if j < len(t.Align) {
				align = t.Align[j]
			}
			x := r.X + l.colX[j]
			switch align {
			case Center:
				x += l.colW[j] / 2
			case Right:
				x += l.colW[j] - l.padX
			default:
				x += l.padX
			}
			block := linesHeight(len(ls), l.size, DefaultLeading)
			y := r.Y + l.rowY[row] + l.padY + (l.rowH[row]-2*l.padY-block)/2
			// The rise can't leave the row: the fit budgets only the settled
			// text, and nothing clips the glyphs, so a bigger rise would paint
			// over the neighbouring row or beyond the rect.
			slack := l.rowY[row] + l.rowH[row] - (y - r.Y + block)
			Text{Font: f, Size: l.size, Color: col, Align: align,
				FX: tableRise(s-0.04*float64(j), 0.3, min(size*0.4, max(slack, 0)))}.Draw(p, strings.Join(ls, "\n"), x, y)
		}
	}
	return r.W, l.h
}

// tableRise fades a cell in over dur seconds while it rises by up to rise
// pixels, as FadeUp does with a rise fixed at 0.4 of the size.
func tableRise(t, dur, rise float64) GlyphEffect {
	p := Ease(t, dur)
	return func(int) GlyphFX { return GlyphFX{DY: (1 - p) * rise, Alpha: p} }
}

// rowStep is the step at which body row i appears.
func (t Table) rowStep(i int) int {
	if t.Reveal == TableRevealRows {
		return t.FirstStep + i
	}
	return t.FirstStep
}

// weight is how emphasized body row i is, 0 to 1: the row the highlight is
// leaving fades down as the one it is arriving at fades up.
func (h tableHighlight) weight(i int) float64 {
	switch i {
	case h.to:
		return h.e
	case h.from:
		return 1 - h.e
	}
	return 0
}

func (t Table) drawHighlight(c Ctx, p *Pixels, r Rect, l *tableLayout, hl tableHighlight, off int) {
	th := c.Theme
	y, rh := r.Y+l.rowY[hl.to+off], l.rowH[hl.to+off]
	alpha := 1.0
	if hl.from >= 0 {
		y = Lerp(r.Y+l.rowY[hl.from+off], y, hl.e)
		rh = Lerp(l.rowH[hl.from+off], rh, hl.e)
	} else {
		alpha = hl.e
	}
	p.Rect(r.X, y, r.W, rh, Mix(th.Background, th.Accent, 0.16), alpha)
	p.Rect(r.X, y, max(c.Unit(0.008), 2), rh, th.Accent, alpha)
}
