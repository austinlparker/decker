package main

import (
	"strings"

	"github.com/austinlparker/decker"
)

// theme is the talk's palette and typefaces, and an overlay with the section
// and page number on every slide.
var theme = &decker.Theme{
	Background: decker.Hex("#0E1420"),
	Text:       decker.Hex("#EEF2F8"),
	Muted:      decker.Hex("#94A0B8"),
	Faint:      decker.Hex("#2A3350"),
	Accent:     decker.Hex("#5AD2FF"),
	Accent2:    decker.Hex("#FF8A3D"),
	Warn:       decker.Hex("#FF5C7A"),
	Good:       decker.Hex("#7BE07B"),
	Panel:      decker.Hex("#141C2E"),
	Display:    decker.StockFont("SpaceGrotesk-Bold"),
	Body:       decker.StockFont("SpaceGrotesk-Medium"),
	Mono:       decker.StockFont("JetBrainsMono-ExtraBold"),
	Overlay: func(c decker.Ctx, p *decker.Pixels) {
		y := c.Y(0.91)
		if c.Section != "" {
			decker.Label(c, p, strings.ToUpper(c.Section), c.X(0.04), y, c.Theme.Muted, decker.Left)
		}
		decker.PageNumber(c, p, c.X(0.96), y, decker.Right, c.Theme.Muted)
	},
}

// heading draws a slide's title and returns the area left below it, above
// the overlay's footer line.
func heading(c decker.Ctx, p *decker.Pixels, title string) decker.Rect {
	th := c.Theme
	page := decker.NewRect(c.X(0.04), c.Y(0.06), c.X(0.92), c.Y(0.82))
	head, body := page.CutTop(c.Y(0.18))
	size, s := th.Display.Fit(title, head.W, head.H, c.Size(0.11), 0)
	decker.Text{Font: th.Display, Size: size, Color: th.Text, FX: decker.RiseIn(c.T, 0.02, size)}.
		Draw(p, s, head.X, head.Y)
	return body
}
