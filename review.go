package decker

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Severity ranks an [Issue]: how much of what a slide meant to show is
// missing or worse than intended.
type Severity int

const (
	// SeverityInfo is worth knowing, though nothing on screen is wrong.
	SeverityInfo Severity = iota
	// SeverityWarning is a slide that shows everything, but worse than
	// intended: text too small to read from the back, say.
	SeverityWarning
	// SeverityError is content lost: lines clipped, rows dropped, text off
	// the canvas, a panic.
	SeverityError
)

// String returns "info", "warning" or "error".
func (s Severity) String() string {
	switch s {
	case SeverityWarning:
		return "warning"
	case SeverityError:
		return "error"
	}
	return "info"
}

// Issue is a problem a review found in one frame: a build of a slide,
// settled, at one size. Code is a stable name for the kind of problem, such
// as "code-clipped", "text-small" or "overflow"; Msg says what and by how
// much, naming the element that has it.
type Issue struct {
	Slide, Step int    // 0-based, like Ctx.Index and Ctx.Step
	Title       string // the slide's title
	W, H        int    // the frame's size in cells

	Severity Severity
	Code     string
	Rect     Rect // where on the canvas, in pixels; zero for the whole frame
	Msg      string
}

// String formats the issue as one line, with the 1-based slide and build
// numbers the command line uses: "11.2 Collector config 320x90 error
// code-clipped: Code "otel.yaml": 5 of 10 lines visible".
func (i Issue) String() string {
	return fmt.Sprintf("%d.%d %s %dx%d %s %s: %s", i.Slide+1, i.Step+1, i.Title, i.W, i.H, i.Severity, i.Code, i.Msg)
}

// ReviewOptions says which frames [Deck.Review] checks.
type ReviewOptions struct {
	// Sizes are the frame sizes to check, in cells, as {W, H}. Nil checks
	// 240×67 (a laptop terminal), 320×90 (a 16:9 window) and 682×171 (a
	// projector at a 4pt font).
	Sizes [][2]int

	// Slides are the 0-based slides to check; nil checks them all.
	Slides []int
}

// reviewSizes are the sizes a review checks unless told otherwise; the guide
// asks for slides that work at all three.
var reviewSizes = [][2]int{{240, 67}, {320, 90}, {682, 171}}

// Review renders every build of every slide, settled, at each size, and
// returns what is wrong with them, in slide, build and size order. Stock
// components report what they cannot fit (a Code block's hidden lines, a
// Table's dropped rows), text reports running off the canvas or being too
// small to read, a panic is an issue rather than a crash, and a slide's own
// drawing reports through [Ctx.Report] and [Ctx.Fits]. Codes a slide lists
// in Slide.Allow are left out.
//
// The frames are the ones the deck shows: Review only listens while they
// are drawn.
func (d *Deck) Review(o ReviewOptions) []Issue {
	sizes := o.Sizes
	if sizes == nil {
		sizes = reviewSizes
	}
	slides := o.Slides
	if slides == nil {
		for i := range d.Slides {
			slides = append(slides, i)
		}
	}
	var out []Issue
	for _, i := range slides {
		for step := range d.Steps(i) {
			for _, sz := range sizes {
				out = append(out, d.reviewFrame(i, step, sz[0], sz[1])...)
			}
		}
	}
	return out
}

// reviewFrame draws slide i at step, settled, at w×h cells, and returns the
// issues it reports, less those the slide allows.
func (d *Deck) reviewFrame(i, step, w, h int) []Issue {
	s := d.Slides[i]
	log := &reviewLog{}
	c := Ctx{W: w, H: h, T: Settled, Step: step, StepT: Settled, Theme: d.Theme, review: log}.at(d.Slides, i)
	renderSlide(s, c).Release()
	log.flush()
	out := log.issues[:0]
	for _, is := range log.issues {
		if slices.Contains(s.Allow, is.Code) {
			continue
		}
		is.Slide, is.Step, is.Title, is.W, is.H = i, step, s.Title, w, h
		out = append(out, is)
	}
	return out
}

// reviewLog collects what one frame reports while it is drawn. Ctx and
// Pixels carry it only under review; while presenting the pointer is nil and
// every check costs one comparison.
type reviewLog struct {
	issues []Issue

	// overlay is set while the theme's overlay draws, so its problems are
	// told apart from the slide's.
	overlay bool

	// scope names the stock component drawing now: its text reports as part
	// of it, since the author chose the component, not each label.
	scope string

	// small gathers text drawn too small to read, one entry per scope (or
	// per text outside one), so a chart with eight tiny labels is one issue.
	small []smallText
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

// enter makes name the owner of what is reported until leave restores the
// scope enter returns: defer l.leave(l.enter("BarChart")).
func (l *reviewLog) enter(name string) string {
	prev := l.scope
	l.scope = name
	return prev
}

func (l *reviewLog) leave(prev string) { l.scope = prev }

// within is enter and leave for a component's Draw, costing nothing while
// presenting: defer c.within("BarChart")().
func (c Ctx) within(name string) func() {
	if c.review == nil {
		return noop
	}
	prev := c.review.enter(name)
	return func() { c.review.leave(prev) }
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
			s.box = unionRect(s.box, box)
			return
		}
	}
	l.small = append(l.small, smallText{owner: owner, n: 1, lo: size, hi: size, least: least, box: box,
		overlay: l.overlay, scoped: scoped, firstOfMany: name})
}

// flush turns the gathered small text into issues, after the frame is drawn.
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
}

// unionRect is the smallest rect holding a and b.
func unionRect(a, b Rect) Rect {
	x0, y0 := min(a.X, b.X), min(a.Y, b.Y)
	return Rect{x0, y0, max(a.Right(), b.Right()) - x0, max(a.Bottom(), b.Bottom()) - y0}
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

// Reviewing reports whether this frame is being checked by [Deck.Review]
// (the -review flag, decktest.Review) rather than shown. Checks that cost
// time belong behind it; [Ctx.Report] and [Ctx.Fits] need no guard.
func (c Ctx) Reviewing() bool { return c.review != nil }

// Report records a problem with what this frame drew in r, for the review
// to list under code (a short stable name, like "waterfall-labels") with
// msg saying what and by how much. It does nothing while presenting; build
// an expensive msg only when [Ctx.Reviewing].
func (c Ctx) Report(sev Severity, code string, r Rect, msg string) {
	if c.review != nil {
		c.review.add(sev, code, r, msg)
	}
}

// Fits reports whether a w×h block fits in r, give or take half a pixel.
// When it doesn't and the frame is under review, it records an "overflow"
// error naming what, with both sizes: custom drawing reports what it can't
// fit the way stock components do.
//
//	if !c.Fits("waterfall", r, layout.W, layout.H) {
//		// drop the duration labels, say
//	}
func (c Ctx) Fits(what string, r Rect, w, h float64) bool {
	ok := w <= r.W+0.5 && h <= r.H+0.5
	if !ok && c.review != nil {
		c.review.add(SeverityError, "overflow", r,
			fmt.Sprintf("%s needs %.0f×%.0fpx, has %.0f×%.0fpx", what, w, h, r.W, r.H))
	}
	return ok
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
// to read, for Text, Rich and Block under review. cov is the block's
// coverage, in canvas coordinates; size is the drawn size in pixels, or 0 for
// block letters, which have no small size.
func checkInk(p *Pixels, cov []coverage, f *Font, size int, name string, box Rect) {
	if size > 0 && f != nil {
		if least := f.Drawn(max(int(MinText*float64(p.H)), 6)); size < least {
			p.review.noteSmall(name, size, least, box)
		}
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
