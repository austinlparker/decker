package decker

// Slide is one slide in the deck. The heart of it is View, a function that
// draws a single frame. It's called once per frame (-fps, default 60) with a
// fresh Ctx, so anything you compute from Ctx.T (seconds since the slide
// appeared) or Ctx.StepT (seconds since the current step began) animates.
//
// View must be a pure function of Ctx: no time.Now, no math/rand (use Hash01
// for repeatable noise), so any frame can be replayed by -snapshot and
// golden tests.
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
