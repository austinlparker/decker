package main

import "github.com/austinlparker/decker"

func main() { decker.Main(talk()) }

func talk() decker.Deck {
	theme := &decker.Theme{
		Background: decker.Hex("#101820"),
		Text:       decker.Hex("#F0F0F0"),
		Muted:      decker.Hex("#A0A8B0"),
		Faint:      decker.Hex("#384048"),
		Accent:     decker.Hex("#40C0FF"),
		Accent2:    decker.Hex("#FFB000"),
		Warn:       decker.Hex("#FF6060"),
		Good:       decker.Hex("#70D050"),
		Panel:      decker.Hex("#0A1016"),
		Display:    decker.StockFont("SpaceGrotesk-Bold"),
		Body:       decker.StockFont("SpaceGrotesk-Medium"),
		Mono:       decker.StockFont("JetBrainsMono-ExtraBold"),
	}
	return decker.Deck{
		Name: "hello-decker", Theme: theme,
		Slides: []decker.Slide{{
			Title: "Hello, decker", Steps: 2,
			Notes: "Press space to reveal the second line. Press q to quit.",
			View: func(c decker.Ctx, sc *decker.Scene) {
				size, title := c.Theme.Display.Fit("Hello, decker", c.X(0.84), c.Y(0.3), c.Size(0.18), 0)
				decker.Text{Font: c.Theme.Display, Size: size, Color: c.Theme.Accent,
					FX: decker.RiseIn(c.T, 0.02, size)}.Draw(sc.Px, title, c.X(0.08), c.Y(0.2))
				if c.Reached(1) {
					decker.Text{Font: c.Theme.Body, Size: c.SmallText(c.Theme.Body), Color: c.Theme.Text,
						FX: decker.FadeUp(c.Since(1), 0.4, c.SmallText(c.Theme.Body))}.
						Draw(sc.Px, "Your slides are Go code.", c.X(0.08), c.Y(0.65))
				}
			},
		}},
	}
}
