package decker

// Transition is how a slide enters. Each kind but Default and None needs an
// entry in transitions.
type Transition int

const (
	TransitionDefault  Transition = iota // use DefaultTransition
	TransitionNone                       // hard cut
	TransitionPush                       // new slide pushes the old one sideways
	TransitionDissolve                   // cells flip from old to new at random
	TransitionWipe                       // a bright edge sweeps across
)

// DefaultTransition applies to slides that don't set Transition.
var DefaultTransition = TransitionPush

// TransitionDuration is how long every transition takes, in seconds.
const TransitionDuration = 0.45

func (k Transition) resolve() Transition {
	if k == TransitionDefault {
		return DefaultTransition
	}
	return k
}

// transitionFunc mixes from into to, in place, at linear progress p from 0
// (all from) to 1 (all to); each applies its own easing. Both are finished
// frames of the same size. A transition moves both of a scene's layers, so
// the terminal, snapshots and video all show the same one: cells are made
// from the result, and video reads its pixels.
type transitionFunc func(from, to *Scene, p float64, forward bool, t *Theme)

var transitions = map[Transition]transitionFunc{
	TransitionPush:     push,
	TransitionDissolve: dissolve,
	TransitionWipe:     wipe,
}

// mixTransition mixes from into to in place. Kinds without an implementation,
// and frames of different sizes, leave to as it is: a cut.
func mixTransition(kind Transition, from, to *Scene, p float64, forward bool, t *Theme) {
	if f, ok := transitions[kind]; ok && from.W == to.W && from.H == to.H {
		f(from, to, p, forward, t)
	}
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
