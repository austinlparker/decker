package decker

import "errors"

// Deck is a talk: its slides in running order, its theme, and a name.
// Pass it to Main from the talk's main function.
type Deck struct {
	// Name identifies the deck on this machine: it names the presenter socket
	// and the binary --dev rebuilds into, and the command line's help shows
	// it as the program's name. Keep it short.
	Name string

	// Theme is required, and so are its Display, Body and Mono fonts.
	Theme *Theme

	// Slides in running order. Must not be empty.
	Slides []Slide

	// Notes are the speaker notes, in Markdown, for the presenter view, the
	// n key and the handout. Each "## " heading starts the next slide's
	// section and is that slide's Title as written, so the file reads as the
	// talk's outline and a slide added or moved without its notes fails
	// the deck's check rather than shifting every note after it. Text
	// before the first heading is about the talk as a whole. Sections may
	// stop before the last slide; the slides after have no notes. Keep
	// them in a file beside the talk and embed it:
	//
	//	//go:embed notes.md
	//	var notes string
	//
	// A fenced block whose info string is "post" is a post for a live
	// thread (Publisher), left out of the notes the speaker sees:
	// media=final (the default) attaches the slide's last build,
	// media=builds an image of each build (builds=2-4,6 picks them),
	// media=clip a looping video of it building, and media=none nothing.
	// An "alt" block right after a post describes its media. Posts before
	// the first section open the thread, and attach nothing.
	Notes string

	// Publisher posts the talk as a thread while it's given, once the
	// presenter view's posting dialog (P) starts it: a slide's posts go
	// out when the speaker moves on from it. Nil leaves the dialog only
	// its rehearsal.
	Publisher Publisher
}

// Render draws slide i (0-based) as a styled string of exactly c.W×c.H cells,
// as the snapshot command prints it. A nil c.Theme uses the deck's, and c.Index, c.Count
// and c.Section are set from i and the deck; a panic renders as its error.
func (d *Deck) Render(i int, c Ctx) string {
	g := d.cells(i, c)
	defer g.release()
	return g.String()
}

// Section returns the section slide i (0-based) belongs to: its own
// Slide.Section, else the nearest one before it, else "".
func (d *Deck) Section(i int) string { return sectionAt(d.Slides, i) }

// Steps returns how many build steps slide i (0-based) has.
func (d *Deck) Steps(i int) int { return d.Slides[i].steps() }

// notes is d.Notes parsed, or no notes at all if it doesn't parse, which
// Check reports.
func (d *Deck) notes() deckNotes {
	dn, err := parseNotes(d.Notes, d.Slides)
	if err != nil {
		return deckNotes{slides: make([]slideNotes, len(d.Slides))}
	}
	return dn
}

// titles are the slides' titles, in order.
func (d *Deck) titles() []string {
	titles := make([]string, len(d.Slides))
	for i, s := range d.Slides {
		titles[i] = s.Title
	}
	return titles
}

// slideNotes is the text of each slide's notes, empty for a slide with none.
func (d *Deck) slideNotes() []string {
	var notes []string
	for _, sn := range d.notes().slides {
		notes = append(notes, sn.text)
	}
	return notes
}

// builds counts the deck's build steps, every slide's together.
func (d *Deck) builds() int {
	n := 0
	for _, s := range d.Slides {
		n += s.steps()
	}
	return n
}

// Draw renders slide i into cells without encoding them: the live deck's
// per-frame work, for tests and benchmarks. Like Render it sets c's position
// from i, and a panic shows in the frame as its error. Draw also returns
// that panic, as an error naming the slide, the part that panicked ("View",
// "a placed element" or "Theme.Overlay") and the panic value, which it wraps
// if it is an error; it returns nil when nothing panicked.
func (d *Deck) Draw(i int, c Ctx) error {
	sc := d.scene(i, c)
	err := sc.err(i)
	sc.flatten().release()
	return err
}

// still renders slide i at step, secs after the slide and the step began,
// at w×h cells: a frame as the snapshot, sheet and handout commands and the
// presenter view's previews show it. The caller releases the grid.
func (d *Deck) still(i, step int, secs float64, w, h int) *grid {
	return d.cells(i, stillCtx(w, h, step, secs))
}

// stillCtx is a w×h frame at step, secs after the slide and the step began;
// Deck.withTheme fills in the rest.
func stillCtx(w, h, step int, secs float64) Ctx {
	return Ctx{W: w, H: h, T: secs, Step: step, StepT: secs}
}

// cells renders slide i as Render does, without encoding it; the caller
// releases the grid.
func (d *Deck) cells(i int, c Ctx) *grid { return d.scene(i, c).flatten() }

// scene renders slide i ready to show (renderSlide) as Render does; the
// caller releases the scene.
func (d *Deck) scene(i int, c Ctx) *Scene { return renderSlide(d.Slides[i], d.withTheme(i, c)) }

// withTheme is c drawing slide i: on the deck's theme unless c has one, and
// at slide i's position in the deck.
func (d *Deck) withTheme(i int, c Ctx) Ctx {
	if c.Theme == nil {
		c.Theme = d.Theme
	}
	return c.at(d.Slides, i)
}

// Check reports what makes d unfit to run, as Main refuses it: a missing
// Name, Theme or theme font, no slides, or Notes whose sections don't follow
// the slides' titles in order.
func (d *Deck) Check() error {
	switch {
	case d.Name == "":
		return errors.New("deck: Name is required")
	case d.Theme == nil:
		return errors.New("deck: Theme is required")
	case d.Theme.Display == nil || d.Theme.Body == nil || d.Theme.Mono == nil:
		return errors.New("deck: Theme needs Display, Body and Mono fonts")
	case len(d.Slides) == 0:
		return errors.New("deck: no slides")
	}
	_, err := parseNotes(d.Notes, d.Slides)
	return err
}
