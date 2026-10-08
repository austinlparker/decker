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
// as -snapshot prints it. c is positioned at i with At; a panic renders as
// its error.
func (d *Deck) Render(i int, c Ctx) string {
	g := renderSlideGrid(d.Slides[i], d.At(i, c))
	defer g.release()
	return g.String()
}

// Section returns the section slide i (0-based) belongs to: its own
// Slide.Section, else the nearest one before it, else "".
func (d *Deck) Section(i int) string { return sectionAt(d.Slides, i) }

// Steps returns how many build steps slide i (0-based) has.
func (d *Deck) Steps(i int) int { return d.Slides[i].steps() }

// Draw renders slide i into cells without encoding them: the live deck's
// per-frame work, for benchmarks. Like Render it positions c with At.
func (d *Deck) Draw(i int, c Ctx) {
	renderSlideGrid(d.Slides[i], d.At(i, c)).release()
}

// At returns c positioned at slide i (0-based), as the engine gives it to
// that slide's View: Index, Count, Section and Sources set from the deck,
// and the deck's Theme if c has none. Tests that call a View themselves
// start from it.
func (d *Deck) At(i int, c Ctx) Ctx {
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
