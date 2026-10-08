package main

import (
	"strings"

	"github.com/austinlparker/decker"
)

// theme is the deck's palette. Series are the colors the recipes give to
// services, in order, through th.SeriesColor.
var theme = &decker.Theme{
	Background: decker.Hex("#0F1218"),
	Text:       decker.Hex("#ECEFF4"),
	Muted:      decker.Hex("#9AA4B5"),
	Faint:      decker.Hex("#2B3242"),
	Accent:     decker.Hex("#6CC6FF"),
	Accent2:    decker.Hex("#FFB86B"),
	Warn:       decker.Hex("#FF6B81"),
	Good:       decker.Hex("#7EDC9A"),
	Panel:      decker.Hex("#181D27"),
	Series: []decker.RGB{
		decker.Hex("#6CC6FF"),
		decker.Hex("#7EDC9A"),
		decker.Hex("#FFB86B"),
		decker.Hex("#C49BFF"),
		decker.Hex("#F2D36B"),
	},
	Display: decker.StockFont("SpaceGrotesk-Bold"),
	Body:    decker.StockFont("SpaceGrotesk-Medium"),
	Mono:    decker.StockFont("JetBrainsMono-ExtraBold"),
	Overlay: func(c decker.Ctx, p *decker.Pixels) {
		th := c.Theme
		decker.ProgressBar(c, p, decker.NewRect(0, c.PH()-c.Unit(0.008), c.PW(), c.Unit(0.008)), th.Accent, th.Faint)
		if c.Section != "" {
			decker.Label(c, p, strings.ToUpper(c.Section), c.X(0.04), c.Y(0.9), th.Muted, decker.Left)
		}
		decker.PageNumber(c, p, c.X(0.96), c.Y(0.9), decker.Right, th.Muted)
	},
}

// heading draws a slide's title and returns the rect left for the slide's
// content: the page inside its margins, below the title and above the
// footer the overlay draws. Recipes lay out from that rect, so the title
// template can change without touching them.
func heading(c decker.Ctx, p *decker.Pixels, title string) decker.Rect {
	th := c.Theme
	page := c.Frame().Inset(c.X(0.04), c.Y(0.05))
	_, page = page.CutBottom(c.Y(0.07)) // the footer row
	head, body := page.CutTop(c.Y(0.11))
	t := decker.Text{Font: th.Display, Size: c.Size(0.095), Color: th.Text}.Fit(title, head.W, head.H)
	t.FX = decker.RiseIn(c.T, 0.02, t.Size)
	_, h := t.Draw(p, title, head.X, head.Y)
	rule := c.X(0.05) * decker.EaseOutExpo(decker.Progress(c.T, 0.1, 0.6))
	p.Rect(head.X, head.Y+h+c.Unit(0.006), rule, c.Unit(0.008), th.Accent, 1)
	_, body = body.CutTop(c.Y(0.03))
	return body
}
