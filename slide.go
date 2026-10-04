package decker

// Slide is one slide in the deck. The heart of it is View, a function that
// draws a single frame. It's called ~60 times per second with a fresh Ctx,
// so anything you compute from Ctx.T (seconds since the slide appeared) or
// Ctx.StepT (seconds since the current step began) animates.
//
// Because a frame depends only on Ctx, slides are replayable and can be
// rendered at any point in time (see the -snapshot flag).
type Slide struct {
	// Title is shown in the footer and window title.
	Title string

	// Steps is how many times "next" stays on this slide before advancing.
	// Use it for builds/reveals: Ctx.Step goes from 0 to Steps-1.
	// Zero means 1.
	Steps int

	// Notes are speaker notes, shown in the presenter view (and on the
	// deck itself with the n key).
	Notes string

	// Hold is how many seconds each build step stays on screen in a video
	// (-video). Zero uses the -hold flag. Set it for slides whose entrance
	// takes longer than that to play out.
	Hold float64

	// Transition is how this slide enters. The zero value uses
	// DefaultTransition.
	Transition Transition

	// HideChrome hides the footer (progress bar, title, counter).
	HideChrome bool

	// View draws one frame. Return a string of any size; it is clipped
	// or padded to Ctx.W x Ctx.H. A panic inside View is caught and shown
	// on screen instead of crashing the deck.
	View func(c Ctx) string
}

func (s Slide) steps() int { return max(s.Steps, 1) }

// Ctx is everything a slide needs to draw a frame.
type Ctx struct {
	W, H  int     // drawable area in cells
	T     float64 // seconds since this slide appeared
	Step  int     // current build step, 0-based
	StepT float64 // seconds since the current step began

	// Theme is the deck's theme. The engine always sets it.
	Theme *Theme
}

// Scene returns an empty Scene the size of the drawable area, on the
// theme's background. Rendering it draws the theme's Overlay on top.
func (c Ctx) Scene() *Scene {
	sc := NewScene(c.W, c.H, c.Theme)
	if o := c.Theme.Overlay; o != nil {
		sc.overlay = func(p *Pixels) { o(c, p) }
	}
	return sc
}

// Reached reports whether the slide is at or past the given step. Handy
// for "show this once step N has been reached" logic.
func (c Ctx) Reached(step int) bool { return c.Step >= step }

// Since returns seconds elapsed since the given step began: negative
// before the step, and a large "settled" value if we are past it. Use it to
// animate the element a step introduces, e.g. Ease(Since(2), 0.5).
func (c Ctx) Since(step int) float64 {
	switch {
	case c.Step < step:
		return -1
	case c.Step == step:
		return c.StepT
	default:
		return Settled
	}
}

// Settled is a time far enough in the future that every entrance animation
// has finished. Slides entered "backwards" (or past steps) use it so they
// appear fully built rather than replaying their animations.
const Settled = 1000.0

// Transition controls how a slide enters.
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

// TransitionDuration is how long transitions take, in seconds.
const TransitionDuration = 0.45
