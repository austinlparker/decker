package decker

// Transition is how a slide enters: one of the kinds below, taking its
// default time or the one Over gives it. Transitions are comparable values;
// the zero Transition is TransitionDefault.
//
//	Transition: decker.TransitionMorph.Over(1.2)
type Transition struct {
	kind transitionKind
	secs float64 // 0 is the kind's default
}

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
	return d
}

// transitionFunc mixes from into to, in place, at linear progress p from 0
// (all from) to 1 (all to); each applies its own easing. Both are frames of
// the same size as drawSlide makes them: View has drawn, and placed elements
// may not be (finish draws them). A transition moves both of a scene's
// layers, so the terminal, snapshots and video all show the same one: cells
// are made from the result, and video reads its pixels.
type transitionFunc func(from, to *Scene, p float64, forward bool, t *Theme)

var transitions = map[transitionKind]transitionFunc{
	kindPush:     finished(push),
	kindDissolve: finished(dissolve),
	kindWipe:     finished(wipe),
	kindMorph:    morph,
}

// finished adapts a transition that mixes two finished frames.
func finished(f transitionFunc) transitionFunc {
	return func(from, to *Scene, p float64, forward bool, t *Theme) {
		from.finish()
		to.finish()
		f(from, to, p, forward, t)
	}
}

// mixTransition mixes from into to in place, leaving to finished. tr is
// resolved already; kinds without an implementation (the default kind
// among them), and frames of different sizes, cut to to.
func mixTransition(tr Transition, from, to *Scene, p float64, forward bool, t *Theme) {
	if f, ok := transitions[tr.kind]; ok && from.W == to.W && from.H == to.H {
		f(from, to, p, forward, t)
	}
	to.finish()
}

func push(from, to *Scene, p float64, forward bool, _ *Theme) {
	w := to.W
	off := LerpInt(0, w, EaseInOutCubic(p))
	// Going forward, the old frame moves left by off and the new one follows
	// it in from the right; going back, both move right.
	shiftOld, shiftNew := -off, w-off
	if !forward {
		shiftOld, shiftNew = off, off-w
	}
	for y := range to.Px.H {
		row, old := to.Px.Pix[y*w:(y+1)*w], from.Px.Pix[y*w:(y+1)*w]
		if forward {
			copy(row[w-off:], row[:off])
			copy(row[:w-off], old[off:])
		} else {
			copy(row[:off], row[w-off:])
			copy(row[off:], old[:w-off])
		}
	}
	moveChars(from, to, func(x, y int, old bool) (int, int, bool) {
		if old {
			x += shiftOld
		} else {
			x += shiftNew
		}
		return x, y, x >= 0 && x < w
	})
}

// dissolve flips cells at random. In a frame wider than 400 cells (video) it
// flips blocks of them, so a dissolve looks the same at any resolution.
func dissolve(from, to *Scene, p float64, _ bool, _ *Theme) {
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
	moveChars(from, to, func(x, y int, fromOld bool) (int, int, bool) { return x, y, old(x, y) == fromOld })
}

// wipe sweeps an edge across in the theme's accent, fading to the background
// over a band a sixtieth of the width; going back, it sweeps the other way.
func wipe(from, to *Scene, p float64, forward bool, t *Theme) {
	w := to.W
	band := float64(w) / 60
	edge := -band + (float64(w)+2*band)*EaseInOutCubic(p)
	// behind is how far column x is behind the edge: below 0 still shows the
	// old frame, band or more the new one, and in between the band.
	behind := func(x int) float64 {
		if !forward {
			x = w - 1 - x
		}
		return edge - float64(x)
	}
	pix, src := to.Px.Pix, from.Px.Pix
	for x := range w {
		d := behind(x)
		if d >= band {
			continue
		}
		col := Mix(t.Accent, t.Background, d/band)
		for k := x; k < len(pix); k += w {
			if d < 0 {
				pix[k] = src[k]
			} else {
				pix[k] = col
			}
		}
	}
	moveChars(from, to, func(x, y int, old bool) (int, int, bool) {
		if old {
			return x, y, behind(x) < 0
		}
		return x, y, behind(x) >= band
	})
}
