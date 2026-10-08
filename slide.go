package decker

import "cmp"

// Slide is one slide in the deck. View draws a frame and is called once per
// frame (-fps) with a fresh Ctx, so anything computed from Ctx.T or Ctx.StepT
// animates. View must be a pure function of Ctx: no time.Now or math/rand
// (use Hash01 for noise), so -snapshot and golden tests can replay any frame.
type Slide struct {
	// Title is shown in the footer, the window title and -list.
	Title string

	// Steps is the number of build states: Ctx.Step runs from 0 to Steps-1.
	// For Steps: 2, one "next" reveals step 1 and the next advances to the
	// following slide. Zero means 1.
	Steps int

	// Notes are speaker notes, shown in the presenter view and with the n key.
	Notes string

	// Sources are the works this slide cites, kept here rather than in the
	// notes so every place that shows them agrees: the presenter view and
	// the n key list them under the notes, -handout and -list -json export
	// them, and Ctx.Sources hands them to View and Theme.Overlay for an
	// on-slide citation.
	Sources []Source

	// Hold is how many seconds each build step stays on screen in a video.
	// Zero uses the -hold flag.
	Hold float64

	// Transition is how this slide enters, and for how long
	// (TransitionWipe.Over(0.6)); zero uses DefaultTransition. The ones that
	// move or sweep take a side (TransitionPush.From(DirUp)). Going back to
	// it plays it again, backwards: from the opposite side.
	Transition Transition

	// Section names the chapter this slide belongs to. A slide with no
	// Section inherits the nearest one before it, so only the first slide of
	// a chapter needs to set it. Ctx.Section is the resolved name, and -list
	// and the presenter view show it.
	Section string

	// Allow lists review issue codes this slide means to have, which
	// Deck.Review then leaves out: "text-offcanvas" for a headline that
	// bleeds off the edge on purpose, say.
	Allow []string

	// HideChrome hides the dev-mode footer on this slide. Theme.Overlay still
	// draws, and presenter notes and previews remain available.
	HideChrome bool

	// View draws one frame onto sc, a Ctx.W x Ctx.H scene already filled
	// with the theme's background; after it returns, the engine draws the
	// elements it placed (Scene.Place), then Theme.Overlay. View must not
	// Render or Release sc. A nil View is a blank
	// slide, and a panic is caught and drawn on screen.
	View func(c Ctx, sc *Scene)
}

// Source is one work a slide cites, in Slide.Sources. Its JSON form is what
// -list -json prints.
type Source struct {
	// Label is what is cited, as a reader should see it: "OpenTelemetry
	// Logs Data Model, v1.40".
	Label string `json:"label"`

	// URL is where to find it; optional. Exports link Label to it.
	URL string `json:"url,omitempty"`
}

// name is what to call s: its Label, or its URL when it has no label.
func (s Source) name() string { return cmp.Or(s.Label, s.URL) }

// line is s as one line of plain text: "Label — URL", or just its name.
func (s Source) line() string {
	if s.Label == "" || s.URL == "" {
		return s.name()
	}
	return s.Label + " — " + s.URL
}

// sectionAt resolves slide i's section: its own, else the nearest non-empty
// one before it.
func sectionAt(slides []Slide, i int) string {
	for ; i >= 0; i-- {
		if s := slides[i].Section; s != "" {
			return s
		}
	}
	return ""
}

func (s Slide) steps() int { return max(s.Steps, 1) }
