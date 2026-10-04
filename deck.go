package decker

import "errors"

// Deck is a talk: its slides in running order, its theme, and a name.
// Pass it to Main from the talk's main function.
type Deck struct {
	// Name identifies the deck on this machine: it names the socket the
	// presenter view links over and the binary dev mode rebuilds into.
	// Keep it short, like "mcp-o11y-talk".
	Name string

	// Theme is the deck's colors, typefaces and overlay. Required, and so
	// are its Display, Body and Mono fonts.
	Theme *Theme

	// Slides in running order. Must not be empty.
	Slides []Slide
}

// Render draws one frame of slide i (0-based) as a styled string of exactly
// c.W×c.H cells: what -snapshot prints, and what tests compare. If c.Theme
// is nil, the deck's theme is used. A panicking slide renders as its error.
func (d *Deck) Render(i int, c Ctx) string {
	return renderSlide(d.Slides[i], d.withTheme(c))
}

// Steps returns how many build steps slide i (0-based) has.
func (d *Deck) Steps(i int) int { return d.Slides[i].steps() }

// Draw draws one frame of slide i into cells without encoding it for a
// terminal: the per-frame work of the live deck, for benchmarks.
func (d *Deck) Draw(i int, c Ctx) {
	renderSlideGrid(d.Slides[i], d.withTheme(c)).release()
}

func (d *Deck) withTheme(c Ctx) Ctx {
	if c.Theme == nil {
		c.Theme = d.Theme
	}
	return c
}

// check reports what is missing from a deck before anything runs.
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
