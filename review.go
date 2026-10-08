package decker

import (
	"fmt"
	"slices"
)

// Severity ranks an [Issue]: how much of what a slide meant to show is
// missing or worse than intended.
type Severity int

const (
	// SeverityWarning is a slide that shows everything, but worse than
	// intended: text too small to read from the back, two things on top of
	// each other.
	SeverityWarning Severity = iota
	// SeverityError is content lost: lines clipped, rows dropped, text off
	// the canvas, a panic.
	SeverityError
)

// String returns "warning" or "error".
func (s Severity) String() string {
	if s == SeverityError {
		return "error"
	}
	return "warning"
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

// reviewSizes are the sizes a review checks: a laptop terminal, a 16:9
// window and a projector at a 4pt font, the three the guide asks slides to
// work at.
var reviewSizes = [][2]int{{240, 67}, {320, 90}, {682, 171}}

// Review renders every build of every slide, settled, at 240×67, 320×90 and
// 682×171 cells, and returns what is wrong with them, in slide, build and
// size order. Stock components report what they cannot fit (a Code block's
// hidden lines, a Table's dropped rows), text reports running off the
// canvas, being too small to read or running into other text, a panic is an
// issue rather than a crash, and a slide's own drawing reports through
// [Ctx.Fits]. Codes a slide lists in Slide.Allow are left out. The frames are
// the ones the deck shows: Review only listens while they are drawn.
func (d *Deck) Review() []Issue {
	var out []Issue
	d.review(reviewSizes, func(f *reviewedFrame) { out = append(out, f.issues...) })
	return out
}

// reviewedFrame is one frame of a review, with what it reported. It is
// only valid inside the review callback that receives it.
type reviewedFrame struct {
	slide, step, w, h int
	issues            []Issue
	sc                *Scene
	elems             []reviewElem
}

// review draws every build of every slide at each size under review, in
// order, and hands each frame to each.
func (d *Deck) review(sizes [][2]int, each func(*reviewedFrame)) {
	for i := range d.Slides {
		for step := range d.Steps(i) {
			for _, sz := range sizes {
				f := d.reviewFrame(i, step, sz)
				each(f)
				f.sc.Release()
			}
		}
	}
}

// reviewFrame draws slide i at step, settled, at sz cells, and keeps the
// issues it reports that the slide doesn't allow.
func (d *Deck) reviewFrame(i, step int, sz [2]int) *reviewedFrame {
	s := d.Slides[i]
	log := newReviewLog(sz[0], 2*sz[1])
	c := d.withTheme(i, stillCtx(sz[0], sz[1], step, Settled))
	c.review = log
	f := &reviewedFrame{slide: i, step: step, w: sz[0], h: sz[1], sc: renderSlide(s, c)}
	log.flush()
	f.elems = log.elems
	for _, is := range log.issues {
		if slices.Contains(s.Allow, is.Code) {
			continue
		}
		is.Slide, is.Step, is.Title, is.W, is.H = i, step, s.Title, sz[0], sz[1]
		f.issues = append(f.issues, is)
	}
	return f
}

// Fits reports whether a w×h block fits in r, give or take half a pixel.
// When it doesn't and the frame is under review, it records an "overflow"
// error naming what, with both sizes: custom drawing reports what it can't
// fit the way stock components do.
//
// Call it on the layout a slide draws, not on one it only tries: a slide
// that falls back to a smaller layout compares sizes itself, then checks the
// one it keeps.
//
//	lay := layoutWaterfall(spans, r)
//	if lay.W > r.W || lay.H > r.H {
//		lay = layoutWaterfall(spans[:8], r) // the tail, rather than clipping
//	}
//	c.Fits("waterfall", r, lay.W, lay.H)
func (c Ctx) Fits(what string, r Rect, w, h float64) bool {
	ok := w <= r.W+0.5 && h <= r.H+0.5
	if !ok && c.review != nil {
		c.review.add(SeverityError, "overflow", r,
			fmt.Sprintf("%s needs %.0f×%.0fpx, has %.0f×%.0fpx", what, w, h, r.W, r.H))
	}
	return ok
}
