package decker

import "errors"

// Deck is a talk: its slides in running order, its theme, and a name.
// Pass it to Main from the talk's main function.
type Deck struct {
	// Name identifies the deck on this machine: it names the presenter socket
	// and the binary -dev rebuilds into. Keep it short.
	Name string

	// Theme is required, and so are its Display, Body and Mono fonts.
	Theme *Theme

	// Slides in running order. Must not be empty.
	Slides []Slide
}

// Render draws slide i (0-based) as a styled string of exactly c.W×c.H cells,
// as -snapshot prints it. A nil c.Theme uses the deck's, and c.Index, c.Count
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
// at w×h cells: a frame as -snapshot, -sheet, the handout and the presenter
// view's previews show it. The caller releases the grid.
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

func (d *Deck) check() error {
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
	return nil
}
