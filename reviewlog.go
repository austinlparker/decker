package decker

import (
	"cmp"
	"fmt"
	"maps"
	"math"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

// reviewLog collects what one frame reports while it is drawn. Ctx and
// Pixels carry it only under review; while presenting the pointer is nil and
// every check costs one comparison.
type reviewLog struct {
	issues []Issue

	// overlay is set while the theme's overlay draws, so its problems are
	// told apart from the slide's.
	overlay bool

	// scope names the stock component or placed element drawing now, and
	// scopeID its element: what it draws reports and overlaps as part of it,
	// since the author chose the component, not each of its labels.
	scope   string
	scopeID int32

	// small gathers text drawn too small to read, one entry per scope (or
	// per text outside one), so a chart with eight tiny labels is one issue.
	small []smallText

	// owner says which element last inked each pixel of the canvas (w×h):
	// 0 for none, else an index+1 into elems. Overlaps between elements are
	// found from it.
	w, h     int
	owner    []int32
	elems    []reviewElem
	overlaps map[[2]int32]int

	// layer is set while a Composite draws on a layer of its own: what is
	// reported there is in the layer's coordinates, and layer puts it where
	// the composite shows it. mute is set while the composite draws the same
	// element again, to learn its coverage, which reports nothing.
	layer *layerMap
	mute  bool
}

// layerMap is how a Composite moves its layer onto the canvas: scaled by s
// about (px, py), then moved by (dx, dy). Only the layer's src box (pixel
// indexes, inclusive) is composited, within the window win on the canvas
// (left, top, right, bottom). outer is the map of a composite drawing this
// one onto a layer of its own.
type layerMap struct {
	s, px, py, dx, dy float64
	src               [4]int
	win               [4]float64
	outer             *layerMap
}

// mapLayer has what is reported, until the returned func runs, land as m
// says, inside any composite already drawing.
func (l *reviewLog) mapLayer(m layerMap) func() {
	m.outer = l.layer
	l.layer = &m
	return func() { l.layer = m.outer }
}

// move is where (x, y) on m's layer lands on the canvas, or on the layer
// of the composite m draws on.
func (m *layerMap) move(x, y float64) (float64, float64) {
	return m.px + (x-m.px)*m.s + m.dx, m.py + (y-m.py)*m.s + m.dy
}

// point is where layer point (x, y) lands on the canvas.
func (m *layerMap) point(x, y float64) (float64, float64) {
	for ; m != nil; m = m.outer {
		x, y = m.move(x, y)
	}
	return x, y
}

// rect is where layer rect r lands on the canvas.
func (m *layerMap) rect(r Rect) Rect {
	x0, y0 := m.point(r.X, r.Y)
	x1, y1 := m.point(r.Right(), r.Bottom())
	return Rect{x0, y0, x1 - x0, y1 - y0}
}

// size is a text size drawn on the layer as it shows on the canvas.
func (m *layerMap) size(size int) int {
	s := float64(size)
	for ; m != nil; m = m.outer {
		s *= m.s
	}
	return int(s)
}

// pixel is the part of the canvas layer pixel (x, y) lands on, or false if
// the composite leaves it out.
func (m *layerMap) pixel(x, y int) (Rect, bool) {
	r := Rect{float64(x), float64(y), 1, 1}
	for ; m != nil; m = m.outer {
		if cx, cy := r.X+r.W/2, r.Y+r.H/2; cx < float64(m.src[0]) || cy < float64(m.src[1]) ||
			cx > float64(m.src[2]+1) || cy > float64(m.src[3]+1) {
			return Rect{}, false
		}
		x0, y0 := m.move(r.X, r.Y)
		x1, y1 := m.move(r.Right(), r.Bottom())
		r = intersectRect(Rect{x0, y0, x1 - x0, y1 - y0}, Rect{m.win[0], m.win[1], m.win[2] - m.win[0], m.win[3] - m.win[1]})
		if r.W <= 0 || r.H <= 0 {
			return Rect{}, false
		}
	}
	return r, true
}

// reviewElem is something a frame drew: a stock component, a block of text
// drawn on its own, or a placed element. Overlaps are between elements, and
// review images outline them.
type reviewElem struct {
	name string
	r    Rect
	ink  int // pixels it inked

	// For a block of text drawn on its own: the whole text, which tells a
	// shadow or an outline (the same text drawn again) from other text, and
	// for Text and Rich its size and the box of each line as aligned.
	text  string
	size  int
	lines []Rect
}

// newReviewLog returns a log for a canvas of w×h pixels.
func newReviewLog(w, h int) *reviewLog {
	return &reviewLog{w: w, h: h, owner: make([]int32, w*h)}
}

type smallText struct {
	owner       string
	n           int
	lo, hi      int // the sizes seen
	least       int // the smallest readable size
	box         Rect
	overlay     bool
	scoped      bool
	firstOfMany string // the first text, for a scope's message
}

// element records an element and returns its id for inkAt, or 0 while
// muted.
func (l *reviewLog) element(name string, r Rect) int32 {
	if l.mute {
		return 0
	}
	if l.layer != nil {
		r = l.layer.rect(r)
	}
	l.elems = append(l.elems, reviewElem{name: name, r: r})
	return int32(len(l.elems))
}

// inkAt records that element id inked pixel (x, y), counting an overlap
// with whatever element inked it before. Id 0 is no element: the overlay's
// drawing, which owns no pixels.
func (l *reviewLog) inkAt(id int32, x, y int) {
	if id == 0 || l.mute {
		return
	}
	if l.layer != nil {
		r, ok := l.layer.pixel(x, y)
		if !ok {
			return
		}
		for cy := int(math.Floor(r.Y)); float64(cy) < r.Bottom(); cy++ {
			for cx := int(math.Floor(r.X)); float64(cx) < r.Right(); cx++ {
				l.inkCanvas(id, cx, cy)
			}
		}
		return
	}
	l.inkCanvas(id, x, y)
}

// inkCanvas is inkAt for a pixel of the canvas itself.
func (l *reviewLog) inkCanvas(id int32, x, y int) {
	if x < 0 || y < 0 || x >= l.w || y >= l.h {
		return
	}
	i := y*l.w + x
	o := l.owner[i]
	if o == id {
		return
	}
	if o != 0 {
		if l.overlaps == nil {
			l.overlaps = map[[2]int32]int{}
		}
		l.overlaps[[2]int32{min(o, id), max(o, id)}]++
	}
	l.owner[i] = id
	l.elems[id-1].ink++
}

// inkRect records element id inking every pixel of r.
func (l *reviewLog) inkRect(id int32, r Rect) {
	for y := max(int(r.Y), 0); y < min(int(r.Bottom()), l.h); y++ {
		for x := max(int(r.X), 0); x < min(int(r.Right()), l.w); x++ {
			l.inkAt(id, x, y)
		}
	}
}

// within makes the component name, drawing in r, the owner of what is drawn
// and reported until the returned func runs: defer c.within("BarChart", r)().
// A component drawn inside another belongs to the outer one. It costs
// nothing while presenting.
func (c Ctx) within(name string, r Rect) func() {
	l := c.review
	if l == nil || l.overlay || l.mute || l.scope != "" {
		return noop
	}
	l.scope, l.scopeID = name, l.element(name, r)
	return func() { l.scope, l.scopeID = "", 0 }
}

func noop() {}

// noteSmall records text drawn at size where least is the smallest readable
// size.
func (l *reviewLog) noteSmall(name string, size, least int, box Rect) {
	if l.mute {
		return
	}
	if l.layer != nil {
		size, box = l.layer.size(size), l.layer.rect(box)
	}
	owner, scoped := name, l.scope != ""
	if scoped {
		owner = l.scope
	}
	for i := range l.small {
		if s := &l.small[i]; s.owner == owner && s.overlay == l.overlay {
			s.n++
			s.lo, s.hi = min(s.lo, size), max(s.hi, size)
			s.box = unionRect(s.box, box)
			return
		}
	}
	l.small = append(l.small, smallText{owner: owner, n: 1, lo: size, hi: size, least: least, box: box,
		overlay: l.overlay, scoped: scoped, firstOfMany: name})
}

// flush turns what was gathered while the frame drew (small text, overlaps)
// into issues.
func (l *reviewLog) flush() {
	for _, s := range l.small {
		sizes := fmt.Sprintf("%dpx", s.lo)
		if s.hi != s.lo {
			sizes = fmt.Sprintf("%d–%dpx", s.lo, s.hi)
		}
		var msg string
		switch {
		case !s.scoped:
			msg = fmt.Sprintf("%s is %s", s.owner, sizes)
		case s.n == 1:
			msg = fmt.Sprintf("%s: %s is %s", s.owner, s.firstOfMany, sizes)
		default:
			msg = fmt.Sprintf("%s: %d texts at %s, such as %s", s.owner, s.n, sizes, s.firstOfMany)
		}
		l.overlay = s.overlay
		l.add(SeverityWarning, "text-small", s.box, msg+fmt.Sprintf("; the smallest readable size here is %dpx", s.least))
	}
	l.overlay, l.small = false, nil

	pairs := slices.SortedFunc(maps.Keys(l.overlaps), func(a, b [2]int32) int {
		return cmp.Or(cmp.Compare(a[0], b[0]), cmp.Compare(a[1], b[1]))
	})
	found := map[[2]int32]bool{}
	for _, k := range pairs {
		n, a, b := l.overlaps[k], l.elems[k[0]-1], l.elems[k[1]-1]
		// A sliver is antialiasing where two things meet.
		if repeated(a, b) || n < 4 || n*20 < min(a.ink, b.ink) {
			continue
		}
		found[k] = true
		l.add(SeverityWarning, "overlap", intersectRect(a.r, b.r), a.name+" and "+b.name+" overlap")
	}
	// Two blocks of text whose lines run into each other share few inked
	// pixels, descenders on capitals, so their line boxes say it instead:
	// two lines sharing a third of a line's height, over a letter's width.
	for i, a := range l.elems {
		for j := i + 1; j < len(l.elems); j++ {
			b := l.elems[j]
			if a.size == 0 || b.size == 0 || repeated(a, b) || found[[2]int32{int32(i + 1), int32(j + 1)}] {
				continue
			}
			if in, ok := linesMeet(a, b); ok {
				l.add(SeverityWarning, "overlap", in, a.name+" and "+b.name+" overlap")
			}
		}
	}
	l.overlaps = nil
}

// repeated reports whether a and b are the same text drawn twice, a shadow
// or an outline, rather than two things in each other's way.
func repeated(a, b reviewElem) bool { return a.text != "" && a.text == b.text }

// linesMeet returns where a line of a meets a line of b, if any pair shares
// a third of the smaller line's height over at least its size in width.
func linesMeet(a, b reviewElem) (Rect, bool) {
	least := float64(min(a.size, b.size))
	for _, la := range a.lines {
		for _, lb := range b.lines {
			if in := intersectRect(la, lb); in.H >= least/3 && in.W >= least {
				return in, true
			}
		}
	}
	return Rect{}, false
}

// unionRect is the smallest rect holding a and b.
func unionRect(a, b Rect) Rect {
	x0, y0 := min(a.X, b.X), min(a.Y, b.Y)
	return Rect{x0, y0, max(a.Right(), b.Right()) - x0, max(a.Bottom(), b.Bottom()) - y0}
}

// intersectRect is where a and b meet, with no width or height if they don't.
func intersectRect(a, b Rect) Rect {
	x0, y0 := max(a.X, b.X), max(a.Y, b.Y)
	return Rect{x0, y0, max(min(a.Right(), b.Right())-x0, 0), max(min(a.Bottom(), b.Bottom())-y0, 0)}
}

// add records an issue, once: an element drawn twice in a frame (a moved
// Composite draws on black and on white) reports twice.
func (l *reviewLog) add(sev Severity, code string, r Rect, msg string) {
	if l.mute {
		return
	}
	if l.layer != nil && r != (Rect{}) {
		r = l.layer.rect(r)
	}
	if l.overlay {
		msg = "Theme.Overlay: " + msg
	} else if l.scope != "" && strings.HasPrefix(code, "text-") {
		msg = l.scope + ": " + msg
	}
	for _, is := range l.issues {
		if is.Code == code && is.Msg == msg && is.Rect == r {
			return
		}
	}
	l.issues = append(l.issues, Issue{Severity: sev, Code: code, Rect: r, Msg: msg})
}

// quoteText names a piece of text in a message: its first line, cut short.
func quoteText(kind, s string) string {
	line, _, cut := strings.Cut(strings.TrimSpace(s), "\n")
	if utf8.RuneCountInString(line) > 28 {
		line, cut = string([]rune(line)[:27]), true
	}
	if cut {
		line += "…"
	}
	return kind + " " + strconv.Quote(line)
}

// checkInk reports text whose ink runs off the canvas or is drawn too small
// to read, for Text and Rich under review, and records its ink for overlaps.
// cov is the block's coverage, in canvas coordinates; size is the drawn size
// in pixels, and lines the box of each line as drawn.
func checkInk(p *Pixels, cov []coverage, f *Font, size int, text, name string, box Rect, lines []Rect) {
	l := p.review
	if l.mute {
		return
	}
	if least := f.Drawn(max(int(MinText*float64(p.H)), 6)); size < least {
		l.noteSmall(name, size, least, box)
	}
	id := l.scopeID
	if id == 0 && !l.overlay {
		id = l.element(name, box)
		e := &l.elems[id-1]
		e.text, e.size, e.lines = text, size, lines
		if l.layer != nil {
			e.size, e.lines = l.layer.size(size), make([]Rect, len(lines))
			for i, r := range lines {
				e.lines[i] = l.layer.rect(r)
			}
		}
	}
	ink := noInk
	for _, c := range cov {
		for y := 0; y < c.h; y++ {
			py := c.y0 + y
			for x := 0; x < c.w; x++ {
				if c.a[y*c.w+x] < 0.5 {
					continue
				}
				px := c.x0 + x
				ink = [4]float64{min(ink[0], float64(px)), min(ink[1], float64(py)), max(ink[2], float64(px+1)), max(ink[3], float64(py+1))}
				if id != 0 {
					l.inkAt(id, px, py)
				}
			}
		}
	}
	l.reportEdges(ink, size, name, box)
}

// noInk is the extent of no ink at all, for reportEdges.
var noInk = [4]float64{math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)}

// reportEdges reports ink reaching past the canvas edges. ink is its extent
// (left, top, right and bottom, in canvas pixels), rounded as its drawing
// rounds. A pixel is rounding; up to a quarter of the text's size is the
// tail of a letter cut off, a warning; more is text lost, an error.
func (l *reviewLog) reportEdges(ink [4]float64, size int, name string, box Rect) {
	if l.mute || ink[0] > ink[2] {
		return
	}
	if l.layer != nil {
		ink[0], ink[1] = l.layer.point(ink[0], ink[1])
		ink[2], ink[3] = l.layer.point(ink[2], ink[3])
		size = l.layer.size(size)
	}
	past := [4]int{
		int(math.Ceil(-ink[0])), int(math.Ceil(-ink[1])),
		int(math.Ceil(ink[2])) - l.w, int(math.Ceil(ink[3])) - l.h,
	}
	edges := [4]string{"left", "top", "right", "bottom"}
	var parts []string
	most := 0
	for i, n := range past {
		if n > 1 {
			parts = append(parts, fmt.Sprintf("%dpx past the %s edge", n, edges[i]))
			most = max(most, n)
		}
	}
	if len(parts) == 0 {
		return
	}
	sev := SeverityError
	if most*4 <= size {
		sev = SeverityWarning
	}
	l.add(sev, "text-offcanvas", box, name+" runs "+strings.Join(parts, " and "))
}
