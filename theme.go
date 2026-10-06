package decker

import "charm.land/lipgloss/v2"

// Theme is the deck's palette and typefaces. The engine paints its chrome
// (background, footer, help, notes, presenter view, wipe) with the colors;
// stock components use the fonts. Display, Body and Mono are required.
type Theme struct {
	Background RGB // painted behind every slide, so the deck looks the same in any terminal
	Text       RGB // default text
	Muted      RGB // secondary text: notes, captions
	Faint      RGB // decoration only: rules, inactive dots
	Accent     RGB // primary highlight: the wipe transition, progress, numbers
	Accent2    RGB // secondary highlight
	Warn       RGB // errors, placeholders
	Good       RGB // positive status, such as the presenter's "on pace"
	Panel      RGB // inset plates and the help box, a shade off Background

	// Series are the colors of chart series, in order, for BarChart, LineChart
	// and DonutChart; see Theme.SeriesColor. Empty uses Accent, Accent2, Good,
	// Warn and Muted.
	Series []RGB

	Display *Font // headlines and numbers
	Body    *Font // supporting lines, labels, chips
	Mono    *Font // code

	// Overlay, if set, draws after every slide into the full canvas p (a TV
	// station's bug), with the slide's Ctx. Slides should leave its corner
	// clear.
	Overlay func(c Ctx, p *Pixels)
}

// styles are the Lip Gloss styles for the engine's own text.
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
