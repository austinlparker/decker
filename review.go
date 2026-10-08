package decker

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
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

func (o ReviewOptions) sizes() [][2]int {
	if o.Sizes == nil {
		return reviewSizes
	}
	return o.Sizes
}

func (o ReviewOptions) slides(n int) []int {
	if o.Slides != nil {
		return o.Slides
	}
	all := make([]int, n)
	for i := range all {
		all[i] = i
	}
	return all
}

// Review renders every build of every slide, settled, at each size, and
// returns what is wrong with them, in slide, build and size order:
//
//   - Content lost: a Code block's hidden lines, a Table's dropped rows,
//     text off the canvas, a block bigger than its rect (stock components
//     and [Ctx.Fits]), a panic. These are errors.
//   - Content hard to read: text below the readable size, two elements
//     inked over each other, the theme's overlay drawn over the slide.
//   - Builds that don't build: a step that looks the same as the one
//     before it, and a TransitionMorph with no element to move.
//   - Frames that aren't pure: a slide that draws differently the second
//     time it is drawn the same way, an error since replay, snapshots, video
//     and goldens all depend on it.
//   - Slides that never settle and redraw a large area every frame, a
//     note, since the terminal must keep up.
//
// A slide's own drawing reports through [Ctx.Report] and [Ctx.Fits]. Codes
// a slide lists in Slide.Allow are left out. The frames are the ones the
// deck shows: Review only listens while they are drawn.
func (d *Deck) Review(o ReviewOptions) []Issue {
	var out []Issue
	d.reviewEach(o, func(f *reviewedFrame) { out = append(out, f.issues...) })
	return out
}

// reviewedFrame is one frame of a review, with what it reported. It is
// only valid inside the reviewEach callback that receives it.
type reviewedFrame struct {
	slide, step, w, h int
	issues            []Issue
	sc                *Scene // nil for an issue about the slide as a whole
	elems             []reviewElem
	log               *reviewLog
}

// reviewEach reviews the frames o names in order, handing each to each.
func (d *Deck) reviewEach(o ReviewOptions, each func(*reviewedFrame)) {
	sizes := o.sizes()
	for _, i := range o.slides(len(d.Slides)) {
		s := d.Slides[i]
		prev := make([]uint64, len(sizes)) // the build before's frame, per size
		var first uint64
		for step := range d.Steps(i) {
			for si, sz := range sizes {
				f := d.reviewFrame(i, step, sz, Settled)
				h := frameHash(f.sc)
				if step == 0 && si == 0 {
					first = h
					d.checkMorph(f)
				}
				if step > 0 && h == prev[si] && d.stepStill(i, step, sz, h) {
					f.log.add(SeverityWarning, "step-unchanged", Rect{},
						fmt.Sprintf("build %d looks the same as build %d, settled and while it enters: check Steps and the step numbers given to c.Reached and c.Since", step+1, step))
				}
				prev[si] = h
				if moving := d.stillMoving(f); moving >= 0.1 {
					f.log.add(SeverityInfo, "never-settles", Rect{},
						fmt.Sprintf("%.0f%% of the frame keeps changing after the slide settles, and the terminal redraws it every frame", moving*100))
				}
				f.done(s)
				each(f)
				f.sc.Release()
			}
		}
		again := d.plainFrame(i, 0, sizes[0], Settled, Settled)
		if frameHash(again) != first {
			f := &reviewedFrame{slide: i, w: sizes[0][0], h: sizes[0][1], log: newReviewLog(0, 0)}
			f.log.add(SeverityError, "impure", Rect{},
				"draws differently the second time it is drawn with the same Ctx: it keeps state between frames or reads the clock or math/rand, which breaks replay, snapshots, video and goldens")
			f.done(s)
			if len(f.issues) > 0 {
				each(f)
			}
		}
		again.Release()
	}
}

// reviewFrame draws slide i at step under review, t seconds in, at sz cells.
func (d *Deck) reviewFrame(i, step int, sz [2]int, t float64) *reviewedFrame {
	log := newReviewLog(sz[0], 2*sz[1])
	c := Ctx{W: sz[0], H: sz[1], T: t, Step: step, StepT: t, Theme: d.Theme, review: log}.at(d.Slides, i)
	sc := renderSlide(d.Slides[i], c)
	log.flush()
	return &reviewedFrame{slide: i, step: step, w: sz[0], h: sz[1], sc: sc, log: log}
}

// done places the frame's issues in the deck and drops those the slide
// allows.
func (f *reviewedFrame) done(s Slide) {
	f.elems = f.log.elems
	for _, is := range f.log.issues {
		if slices.Contains(s.Allow, is.Code) {
			continue
		}
		is.Slide, is.Step, is.Title, is.W, is.H = f.slide, f.step, s.Title, f.w, f.h
		f.issues = append(f.issues, is)
	}
}

// plainFrame draws slide i as the deck shows it, without review.
func (d *Deck) plainFrame(i, step int, sz [2]int, t, stepT float64) *Scene {
	return renderSlide(d.Slides[i], Ctx{W: sz[0], H: sz[1], T: t, Step: step, StepT: stepT, Theme: d.Theme}.at(d.Slides, i))
}

// stepStill reports whether build step of slide i, settled to hash, also
// looks that way while it enters: a step that only plays an animation
// (a pulse, a shake) settles back to the build before, and that's fine.
func (d *Deck) stepStill(i, step int, sz [2]int, hash uint64) bool {
	for _, stepT := range []float64{0.15, 0.4, 1} {
		sc := d.plainFrame(i, step, sz, Settled, stepT)
		h := frameHash(sc)
		sc.Release()
		if h != hash {
			return false
		}
	}
	return true
}

// stillMoving returns the share of f's pixels that differ a moment later.
// The moment is off any round period, so a loop doesn't look still.
func (d *Deck) stillMoving(f *reviewedFrame) float64 {
	const later = Settled + 0.37
	sc := d.plainFrame(f.slide, f.step, [2]int{f.w, f.h}, later, later)
	defer sc.Release()
	n := 0
	for i, c := range sc.Px.Pix {
		if c != f.sc.Px.Pix[i] {
			n++
		}
	}
	return float64(n) / float64(len(sc.Px.Pix))
}

// checkMorph warns when f's slide enters with TransitionMorph but has
// nothing to move: no element placed under a key the slide before placed.
func (d *Deck) checkMorph(f *reviewedFrame) {
	s := d.Slides[f.slide]
	if f.slide == 0 || s.Transition.resolve().kind != kindMorph {
		return
	}
	if len(f.sc.placed) == 0 {
		f.log.add(SeverityWarning, "morph-unmatched", Rect{},
			"enters with TransitionMorph but places no elements (Scene.Place), so it cross-fades")
		return
	}
	prevStep := d.Steps(f.slide-1) - 1
	prev := drawSlide(d.Slides[f.slide-1], Ctx{W: f.w, H: f.h, T: Settled, Step: prevStep, StepT: Settled, Theme: d.Theme}.at(d.Slides, f.slide-1))
	defer prev.Release()
	var mine, theirs []string
	for _, e := range f.sc.placed {
		mine = append(mine, strconv.Quote(e.key))
	}
	for _, e := range prev.placed {
		if slices.Contains(mine, strconv.Quote(e.key)) && e.key != "" {
			return
		}
		theirs = append(theirs, strconv.Quote(e.key))
	}
	f.log.add(SeverityWarning, "morph-unmatched", Rect{},
		fmt.Sprintf("enters with TransitionMorph but shares no Place key with slide %d (it places %s; this slide places %s), so it cross-fades",
			f.slide, orNone(theirs), strings.Join(mine, ", ")))
}

func orNone(keys []string) string {
	if len(keys) == 0 {
		return "none"
	}
	return strings.Join(keys, ", ")
}

// frameHash identifies a finished frame: its pixels and its characters.
func frameHash(sc *Scene) uint64 {
	h := uint64(14695981039346656037)
	mix := func(v uint64) { h = (h ^ v) * 1099511628211 }
	for _, c := range sc.Px.Pix {
		mix(uint64(math.Float32bits(c.R)))
		mix(uint64(math.Float32bits(c.G)))
		mix(uint64(math.Float32bits(c.B)))
	}
	for _, i := range sc.used {
		mix(uint64(i))
		for _, b := range []byte(sc.cells[i].Content) {
			mix(uint64(b))
		}
	}
	return h
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
