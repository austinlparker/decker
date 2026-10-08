package decker

import (
	"cmp"
	"fmt"
	"maps"
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
	// 0 for none, else an index+1 into elems. Overlaps between elements, and
	// the overlay drawing over one, are found from it.
	w, h     int
	owner    []int32
	elems    []reviewElem
	overlaps map[[2]int32]int
}

// reviewElem is something a frame drew: a stock component, a block of text
// drawn on its own, or a placed element. Review images outline them.
type reviewElem struct {
	name string
	r    Rect
	ink  int // pixels it inked
	size int // for Text or Rich drawn on its own, its size in pixels; their boxes are their lines
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

// element records an element and returns its id for inkAt.
func (l *reviewLog) element(name string, r Rect) int32 {
	l.elems = append(l.elems, reviewElem{name: name, r: r})
	return int32(len(l.elems))
}

// inkAt records that element id inked pixel (x, y), counting an overlap
// with whatever element inked it before.
func (l *reviewLog) inkAt(id int32, x, y int) {
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
	if l == nil || l.overlay || l.scope != "" {
		return noop
	}
	l.scope, l.scopeID = name, l.element(name, r)
	return func() { l.scope, l.scopeID = "", 0 }
}

func noop() {}

// noteSmall records text drawn at size where least is the smallest readable
// size.
func (l *reviewLog) noteSmall(name string, size, least int, box Rect) {
	owner, scoped := name, l.scope != ""
	if scoped {
		owner = l.scope
	}
	for i := range l.small {
		if s := &l.small[i]; s.owner == owner && s.overlay == l.overlay {
			s.n++
			s.lo, s.hi = min(s.lo, size), max(s.hi, size)
			s.box = s.box.Union(box)
			return
		}
	}
	l.small = append(l.small, smallText{owner: owner, n: 1, lo: size, hi: size, least: least, box: box,
		overlay: l.overlay, scoped: scoped, firstOfMany: name})
}

// overlayDrew compares the canvas before the theme's overlay drew with
// after, and reports each element the overlay drew over.
func (l *reviewLog) overlayDrew(before, after []RGB) {
	over := map[int32]int{}
	for i := range l.owner {
		if after[i] != before[i] && l.owner[i] != 0 {
			over[l.owner[i]]++
		}
	}
	for _, id := range slices.Sorted(maps.Keys(over)) {
		if e := l.elems[id-1]; over[id] >= 4 {
			l.add(SeverityWarning, "overlay-collision", e.r, "Theme.Overlay draws over "+e.name)
		}
	}
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
		// The same text drawn twice is a shadow or an outline. A sliver is
		// antialiasing where two things meet.
		if a.name == b.name || n < 4 || n*20 < min(a.ink, b.ink) {
			continue
		}
		found[k] = true
		l.add(SeverityWarning, "overlap", a.r.Intersect(b.r), a.name+" and "+b.name+" overlap")
	}
	// Two blocks of text whose lines run into each other share few inked
	// pixels, descenders on capitals, so their line boxes say it instead:
	// sharing a third of a line's height, over at least a letter's width.
	for i, a := range l.elems {
		for j := i + 1; j < len(l.elems); j++ {
			b := l.elems[j]
			k := [2]int32{int32(i + 1), int32(j + 1)}
			if a.size == 0 || b.size == 0 || a.name == b.name || found[k] {
				continue
			}
			least := float64(min(a.size, b.size))
			if in := a.r.Intersect(b.r); in.H >= least/3 && in.W >= least {
				l.add(SeverityWarning, "overlap", in, a.name+" and "+b.name+" overlap")
			}
		}
	}
	l.overlaps = nil
}

// add records an issue, once: an element drawn twice in a frame (a moved
// Composite draws on black and on white) reports twice.
func (l *reviewLog) add(sev Severity, code string, r Rect, msg string) {
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
// in pixels.
func checkInk(p *Pixels, cov []coverage, f *Font, size int, name string, box Rect) {
	l := p.review
	if least := f.Drawn(max(int(MinText*float64(p.H)), 6)); size < least {
		l.noteSmall(name, size, least, box)
	}
	id := l.scopeID
	if id == 0 && !l.overlay {
		id = l.element(name, box)
		l.elems[id-1].size = size
	}
	// How far ink reaches past each edge: left, top, right, bottom.
	var past [4]int
	for _, c := range cov {
		for y := 0; y < c.h; y++ {
			py := c.y0 + y
			for x := 0; x < c.w; x++ {
				if c.a[y*c.w+x] < 0.5 {
					continue
				}
				px := c.x0 + x
				past[0] = max(past[0], -px)
				past[1] = max(past[1], -py)
				past[2] = max(past[2], px-p.W+1)
				past[3] = max(past[3], py-p.H+1)
				if id != 0 {
					l.inkAt(id, px, py)
				}
			}
		}
	}
	reportPast(p, past, size, name, box)
}

// reportPast reports ink that reaches past the canvas edges by the given
// pixels: left, top, right, bottom. A pixel is rounding; up to a quarter of
// the text's size is the tail of a letter cut off, a warning; more is text
// lost, an error.
func reportPast(p *Pixels, past [4]int, size int, name string, box Rect) {
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
	p.review.add(sev, "text-offcanvas", box, name+" runs "+strings.Join(parts, " and "))
}
