package decker

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// Theme is a deck's look, as far as the engine is concerned: the colors it
// paints the background, footer, help, notes, transitions and presenter
// view with; the typefaces its stock components (Panel, Chip, Label,
// CycleDiagram, BulletList…) draw in; and an optional overlay for every
// slide. A deck's own style (title treatment, backdrops, characters) lives
// with the deck and reads the same colors.
//
// Slides reach it as c.Theme.
type Theme struct {
	Background RGB // painted behind every slide, so the deck looks the same in any terminal
	Text       RGB
	Muted      RGB
	Faint      RGB // decoration only: rules, inactive dots
	Accent     RGB // primary highlight: the wipe transition, progress, numbers
	Accent2    RGB // secondary highlight
	Warn       RGB // errors, placeholders
	Good       RGB // the presenter's "on pace"
	Panel      RGB // inset plates and the help box, a shade off Background

	Display *Font // headlines and numbers
	Body    *Font // supporting lines, labels, chips
	Mono    *Font // code

	// Overlay, if set, draws on top of every slide scene, like a TV
	// station's bug in the corner. It gets the slide's Ctx (for sizes,
	// c.SmallText, and the time) and the finished canvas. Slides should
	// leave its corner clear.
	Overlay func(c Ctx, p *Pixels)
}

// styles are the Lip Gloss styles for the engine's own text: footer, help,
// notes, dev status and the presenter view.
type styles struct {
	text, muted, faint, accent, accent2, warn, good lipgloss.Style
}

func (t *Theme) styles() styles {
	fg := func(c RGB) lipgloss.Style { return lipgloss.NewStyle().Foreground(c.Color()) }
	return styles{
		text:    fg(t.Text),
		muted:   fg(t.Muted),
		faint:   fg(t.Faint),
		accent:  fg(t.Accent).Bold(true),
		accent2: fg(t.Accent2).Bold(true),
		warn:    fg(t.Warn).Bold(true),
		good:    fg(t.Good).Bold(true),
	}
}

// Hex converts a "#RRGGBB" string to RGB.
func Hex(s string) RGB { return toRGB(lipgloss.Color(s)) }

// Color returns c as an opaque color.Color, for Lip Gloss.
func (c RGB) Color() color.Color {
	q := c.q()
	return color.RGBA{q[0], q[1], q[2], 255}
}
