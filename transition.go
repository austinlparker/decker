package decker

// Transition is how a slide enters: one of the kinds below, taking its
// default time or the one Over gives it, and the side From gives the ones
// that have a direction. Transitions are comparable values; the zero
// Transition is TransitionDefault.
//
//	Transition: decker.TransitionMorph.Over(1.2)
//	Transition: decker.TransitionPush.From(decker.DirDown)
type Transition struct {
	kind transitionKind
	secs float64 // 0 is the kind's default
	dir  Direction
}

// opposite is the side a transition plays from when going back.
func (d Direction) opposite() Direction {
	switch d {
	case DirRight:
		return DirLeft
	case DirLeft:
		return DirRight
	case DirUp:
		return DirDown
	case DirDown:
		return DirUp
	}
	return d
}

// horizontal reports whether d moves things along the x axis.
func (d Direction) horizontal() bool { return d == DirRight || d == DirLeft }

// transitionKind picks the implementation. Each kind but default and none
// needs an entry in transitions.
type transitionKind int

const (
	kindDefault transitionKind = iota
	kindNone
	kindPush
	kindDissolve
	kindWipe
	kindMorph
	kindFade
	kindFadeThrough
	kindCover
	kindUncover
	kindSplit
	kindIris
	kindZoom
	kindPixelate
	kindGlitch
)

// The transitions. A slide's Transition is one of these, optionally with Over.
var (
	TransitionDefault  = Transition{}                   // DefaultTransition, but Over still sets the time
	TransitionNone     = Transition{kind: kindNone}     // hard cut
	TransitionPush     = Transition{kind: kindPush}     // new slide pushes the old one sideways
	TransitionDissolve = Transition{kind: kindDissolve} // cells flip from old to new at random
	TransitionWipe     = Transition{kind: kindWipe}     // a bright edge sweeps across

	// TransitionMorph moves elements placed with the same key (Scene.Place)
	// to their new rects, and cross-fades the rest. It takes MorphDuration.
	TransitionMorph = Transition{kind: kindMorph}

	TransitionFade        = Transition{kind: kindFade}        // cross-fade; characters switch half-way
	TransitionFadeThrough = Transition{kind: kindFadeThrough} // old fades to the background, then new fades in
	TransitionCover       = Transition{kind: kindCover}       // new slide slides in over the old one, which stays put
	TransitionUncover     = Transition{kind: kindUncover}     // old slide slides away off the new one, which stays put
	TransitionSplit       = Transition{kind: kindSplit}       // old slide opens like barn doors from the center
	TransitionIris        = Transition{kind: kindIris}        // a circle of the new slide grows from the center
	TransitionZoom        = Transition{kind: kindZoom}        // old zooms in and fades out, new grows into place
	TransitionPixelate    = Transition{kind: kindPixelate}    // old breaks into big blocks, new resolves from them
	TransitionGlitch      = Transition{kind: kindGlitch}      // noisy slices jump sideways, split in color, and flip to new
)

// DefaultTransition applies to slides that don't set Transition.
var DefaultTransition = TransitionPush

// TransitionDuration is how long a transition takes by default, in seconds;
// TransitionMorph takes MorphDuration.
const TransitionDuration = 0.45

// MorphDuration is how long TransitionMorph takes by default, in seconds:
// moving things need longer to read than a cut between frames.
const MorphDuration = 0.8

// Over returns t taking secs seconds; zero or less restores its default.
func (t Transition) Over(secs float64) Transition {
	t.secs = max(secs, 0)
	return t
}

// From returns t starting from side d. Push, Cover and Uncover bring the new
// slide in from d (Uncover sends the old one out the opposite way), Wipe
// sweeps from d, and Split opens along d's axis; the other transitions
// ignore it. DirDefault restores each one's own side (Push, Cover and
// Uncover from the right, Wipe from the left, Split horizontal), and going
// back plays t from the opposite side.
func (t Transition) From(d Direction) Transition {
	t.dir = d
	return t
}

// Duration returns how long t takes, in seconds.
func (t Transition) Duration() float64 {
	switch {
	case t.secs > 0:
		return t.secs
	case t.kind == kindDefault:
		return DefaultTransition.resolve().Duration()
	case t.kind == kindMorph:
		return MorphDuration
	}
	return TransitionDuration
}

// resolve replaces the default kind with DefaultTransition's, keeping t's own
// time if it has one.
func (t Transition) resolve() Transition {
	if t.kind != kindDefault {
		return t
	}
	d := DefaultTransition
	if d.kind == kindDefault {
		d = TransitionPush
	}
	if t.secs > 0 {
		d.secs = t.secs
	}
	if t.dir != DirDefault {
		d.dir = t.dir
	}
	return d
}

// side is the side t plays from: its own or its kind's, flipped going back.
// It is the only direction a transitionFunc sees.
func (t Transition) side(forward bool) Direction {
	d := t.dir
	if d == DirDefault {
		d = DirRight
		if t.kind == kindWipe {
			d = DirLeft
		}
	}
	if !forward {
		d = d.opposite()
	}
	return d
}

// transitionFunc mixes from into to, in place, at linear progress p from 0
// (all from) to 1 (all to); each applies its own easing. side is where it
// plays from (Transition.side), which the ones without a direction ignore.
// Both are frames of
// the same size as drawSlide makes them: View has drawn, and placed elements
// may not be (finish draws them). A transition moves both of a scene's
// layers, so the terminal, snapshots and video all show the same one: cells
// are made from the result, and video reads its pixels.
type transitionFunc func(from, to *Scene, p float64, side Direction, t *Theme)

var transitions = map[transitionKind]transitionFunc{
	kindPush:     finished(push),
	kindDissolve: finished(dissolve),
	kindWipe:     finished(wipe),
	kindMorph:    morph,

	kindFade:        finished(crossfade),
	kindFadeThrough: finished(fadeThrough),
	kindCover:       finished(cover),
	kindUncover:     finished(uncover),
	kindSplit:       finished(split),
	kindIris:        finished(iris),
	kindZoom:        finished(zoom),
	kindPixelate:    finished(pixelate),
	kindGlitch:      finished(glitch),
}

// finished adapts a transition that mixes two finished frames.
func finished(f transitionFunc) transitionFunc {
	return func(from, to *Scene, p float64, side Direction, t *Theme) {
		from.finish()
		to.finish()
		f(from, to, p, side, t)
	}
}

// mixTransition mixes from into to in place, leaving to finished. tr is
// resolved already; kinds without an implementation (the default kind
// among them), and frames of different sizes, cut to to.
func mixTransition(tr Transition, from, to *Scene, p float64, forward bool, t *Theme) {
	if f, ok := transitions[tr.kind]; ok && from.W == to.W && from.H == to.H {
		f(from, to, p, tr.side(forward), t)
	}
	to.finish()
}

// span says what a stretch of the slide axis shows in a frame mid-way
// through a push, cover, uncover or split: the cells [lo, hi) show the old
// or the new slide, taken from shift cells back along the axis. Whole cells
// keep the character layer and the pixels together; vertically a cell is two
// pixel rows.
type span struct {
	lo, hi, shift int
	old           bool
}

func push(from, to *Scene, p float64, side Direction, _ *Theme) {
	slide(from, to, p, side, true, true)
}

// cover slides the new frame in over the old one, which stays put.
func cover(from, to *Scene, p float64, side Direction, _ *Theme) {
	slide(from, to, p, side, true, false)
}

// uncover slides the old frame away off the new one, which stays put.
func uncover(from, to *Scene, p float64, side Direction, _ *Theme) {
	slide(from, to, p, side, false, true)
}

// slide places both frames for push, cover and uncover: the new one enters
// from side, the old one leaves the opposite way, and a frame that doesn't
// move stays where it is.
func slide(from, to *Scene, p float64, side Direction, moveNew, moveOld bool) {
	horiz := side.horizontal()
	n := to.H
	if horiz {
		n = to.W
	}
	o := LerpInt(0, n, EaseInOutCubic(p))
	var spans [2]span
	if side == DirRight || side == DirDown {
		spans[0] = span{lo: n - o, hi: n}
		spans[1] = span{lo: 0, hi: n - o, old: true}
		if moveNew {
			spans[0].shift = n - o
		}
		if moveOld {
			spans[1].shift = -o
		}
	} else {
		spans[0] = span{lo: 0, hi: o}
		spans[1] = span{lo: o, hi: n, old: true}
		if moveNew {
			spans[0].shift = o - n
		}
		if moveOld {
			spans[1].shift = o
		}
	}
	place(from, to, horiz, spans[:])
}

// split opens the old frame like barn doors: its halves slide apart, to the
// sides or up and down by side's axis, and the new frame is behind them.
func split(from, to *Scene, p float64, side Direction, _ *Theme) {
	horiz := side.horizontal()
	n := to.H
	if horiz {
		n = to.W
	}
	l := n / 2
	e := EaseInOutCubic(p)
	oL, oR := LerpInt(0, l, e), LerpInt(0, n-l, e)
	spans := [3]span{
		{lo: l - oL, hi: l + oR},
		{lo: 0, hi: l - oL, shift: -oL, old: true},
		{lo: l + oR, hi: n, shift: oR, old: true},
	}
	place(from, to, horiz, spans[:])
}

// place builds to from spans that don't overlap: the parts of the new frame
// that move first, since they read to's own pixels, then the old frame's
// parts (parts of the new frame that stay are in place already); and the
// character layer the same way.
func place(from, to *Scene, horiz bool, spans []span) {
	for _, s := range spans {
		if !s.old && s.shift != 0 {
			copySpan(to.Px, to.Px, horiz, s)
		}
	}
	for _, s := range spans {
		if s.old {
			copySpan(to.Px, from.Px, horiz, s)
		}
	}
	moveChars(from, to, func(x, y int, old bool) (int, int, bool) {
		a := x
		if !horiz {
			a = y
		}
		for _, s := range spans {
			if s.old == old && a >= s.lo-s.shift && a < s.hi-s.shift {
				if horiz {
					return x + s.shift, y, true
				}
				return x, y + s.shift, true
			}
		}
		return 0, 0, false
	})
}

// copySpan copies src's pixels into dst over s; they may be the same pixels,
// which is safe: a row copy handles overlap, and vertical moves go in the
// order that reads each row before it is written.
func copySpan(dst, src *Pixels, horiz bool, s span) {
	w := dst.W
	if horiz {
		for y := range dst.H {
			copy(dst.Pix[y*w+s.lo:y*w+s.hi], src.Pix[y*w+s.lo-s.shift:y*w+s.hi-s.shift])
		}
		return
	}
	lo, hi, shift := 2*s.lo, 2*s.hi, 2*s.shift
	if shift > 0 {
		for y := hi - 1; y >= lo; y-- {
			copy(dst.Pix[y*w:(y+1)*w], src.Pix[(y-shift)*w:(y-shift+1)*w])
		}
		return
	}
	for y := lo; y < hi; y++ {
		copy(dst.Pix[y*w:(y+1)*w], src.Pix[(y-shift)*w:(y-shift+1)*w])
	}
}

// dissolve flips cells at random. In a frame wider than 400 cells (video) it
// flips blocks of them, so a dissolve looks the same at any resolution.
func dissolve(from, to *Scene, p float64, _ Direction, _ *Theme) {
	w := to.W
	bs := max(w/400, 1)
	old := func(x, y int) bool { return Hash01(x/bs, y/bs, 7) >= p }
	pix, src := to.Px.Pix, from.Px.Pix
	for y := range to.H {
		for x := range w {
			if old(x, y) {
				top, bot := 2*y*w+x, (2*y+1)*w+x
				pix[top], pix[bot] = src[top], src[bot]
			}
		}
	}
	keepChars(from, to, func(x, y int, fromOld bool) bool { return old(x, y) == fromOld })
}

// wipe sweeps an edge across in the theme's accent, fading to the background
// over a band a sixtieth of the width, from side; going back, it sweeps from
// the other side. Vertically the band is as many pixel rows as it is columns
// horizontally, and a cell shows a slide only once both its pixel rows do.
func wipe(from, to *Scene, p float64, side Direction, t *Theme) {
	w, horiz := to.W, side.horizontal()
	n := to.Px.H
	if horiz {
		n = w
	}
	band := float64(w) / 60
	edge := -band + (float64(n)+2*band)*EaseInOutCubic(p)
	// behind is how far position a (a column, or a pixel row) is behind the
	// edge: below 0 still shows the old frame, band or more the new one, and
	// in between the band.
	behind := func(a int) float64 {
		if side == DirRight || side == DirDown {
			a = n - 1 - a
		}
		return edge - float64(a)
	}
	pix, src := to.Px.Pix, from.Px.Pix
	for a := range n {
		d := behind(a)
		if d >= band {
			continue
		}
		col := Mix(t.Accent, t.Background, d/band)
		lo, hi, step := a, len(pix), w // a column
		if !horiz {
			lo, hi, step = a*w, (a+1)*w, 1 // a row
		}
		for k := lo; k < hi; k += step {
			if d < 0 {
				pix[k] = src[k]
			} else {
				pix[k] = col
			}
		}
	}
	keepChars(from, to, func(x, y int, old bool) bool {
		d0, d1 := behind(x), behind(x)
		if !horiz {
			d0, d1 = behind(2*y), behind(2*y+1)
		}
		if old {
			return max(d0, d1) < 0
		}
		return min(d0, d1) >= band
	})
}
