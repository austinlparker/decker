package decker

// Slide is one slide in the deck. View draws a frame and is called once per
// frame (-fps) with a fresh Ctx, so anything computed from Ctx.T or Ctx.StepT
// animates. View must be a pure function of Ctx: no time.Now or math/rand
// (use Hash01 for noise), so -snapshot and golden tests can replay any frame.
type Slide struct {
	// Title is shown in the footer, the window title and -list.
	Title string

	// Steps is how many times "next" stays on this slide: Ctx.Step runs from
	// 0 to Steps-1. Zero means 1.
	Steps int

	// Notes are speaker notes, shown in the presenter view and with the n key.
	Notes string

	// Hold is how many seconds each build step stays on screen in a video.
	// Zero uses the -hold flag.
	Hold float64

	// Transition is how this slide enters, and for how long
	// (TransitionWipe.Over(0.6)); zero uses DefaultTransition. Going back to
	// it plays it again, backwards.
	Transition Transition

	HideChrome bool

	// View draws one frame onto sc, a Ctx.W x Ctx.H scene already filled
	// with the theme's background; after it returns, the engine draws the
	// elements it placed (Scene.Place), then Theme.Overlay. View must not
	// Render or Release sc. A nil View is a blank
	// slide, and a panic is caught and drawn on screen.
	View func(c Ctx, sc *Scene)
}

func (s Slide) steps() int { return max(s.Steps, 1) }
