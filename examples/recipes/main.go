// Command recipes is a deck of worked recipes for the visuals that carry a
// talk's explanation: a trace waterfall, an architecture diagram that zooms
// into one service, and a request flow. The engine has stock components for
// routine content (code, tables, charts); visuals like these are better
// purpose-built from primitives, so each recipe here is a small function to
// copy into a talk and edit, not an API to call.
//
// Every recipe follows the same pattern: lay out first, as a pure function of
// a rect and the data; check the layout against the rect with c.Fits, and
// degrade on purpose when it doesn't fit; then draw, revealing over the
// slide's builds. Run the review to see it hold at every size:
//
//	go run ./examples/recipes -review review
package main

import "github.com/austinlparker/decker"

func main() { decker.Main(talk()) }

func talk() decker.Deck {
	return decker.Deck{Name: "decker-recipes", Theme: theme, Slides: []decker.Slide{
		introSlide(),
		waterfallSlide(),
		architectureSlide(),
		architectureZoomSlide(),
		requestFlowSlide(),
	}}
}

// introSlide says what the deck is and how to use a recipe.
func introSlide() decker.Slide {
	return decker.Slide{Title: "Recipes", Section: "Recipes", Hold: 3,
		Notes: "A deck of worked visuals. Each recipe is one file: copy it into your talk, change the data, and run the review until it is clean.",
		View: func(c decker.Ctx, sc *decker.Scene) {
			p, th := sc.Px, c.Theme
			page := c.Frame().Inset(c.X(0.08), c.Y(0.1))
			_, page = page.CutBottom(c.Y(0.08)) // the overlay's footer

			size, s := th.Display.Fit("Decker recipes", page.W, page.H*0.26, c.Size(0.22), 0)
			title := decker.Text{Font: th.Display, Size: size, Color: th.Text, FX: decker.RiseIn(c.T, 0.025, size)}
			_, h := title.Draw(p, s, page.X, page.Y)
			sub := decker.Text{Font: th.Body, Size: c.SmallText(th.Body), Color: th.Muted, MaxW: page.W,
				FX: decker.FadeUp(c.T-0.4, 0.5, c.SmallText(th.Body))}
			_, sh := sub.Draw(p, "Visuals built from primitives, to copy and adapt.", page.X, page.Y+h+c.Unit(0.02))

			// Three numbered steps share what the title and the subtitle
			// (wrapped on a narrow screen) leave, a row each: a number in a
			// disc, then the words. The last is the command, set in the mono
			// face on a plate.
			steps := []string{"Copy a recipe file into your talk", "Change its data and the knobs on top", "go run . -review review"}
			_, rest := page.CutTop(h + c.Unit(0.02) + sh + c.Unit(0.04))
			rows := rest.Rows(c.Unit(0.03), 1, 1, 1)
			txt := decker.Text{Font: th.Body, Size: c.SmallText(th.Body), Color: th.Text}
			mono := decker.Text{Font: th.Mono, Size: c.SmallText(th.Mono), Color: th.Accent}
			for i, row := range rows {
				t := c.T - 0.7 - 0.25*float64(i)
				a := decker.Ease(t, 0.4)
				if a <= 0 {
					continue
				}
				cy := row.Y + row.H/2
				r := row.H * 0.36
				p.Disc(row.X+r, cy, r, decker.Mix(th.Background, th.Accent2, a), 1)
				num := decker.Text{Font: th.Display, Size: c.SmallText(th.Display), Color: decker.Mix(th.Accent2, th.Background, a), Align: decker.Center}
				num.DrawMid(p, string(rune('1'+i)), row.X+r, cy)
				x := row.X + 2*r + c.Unit(0.06)
				if i == len(steps)-1 {
					w, _ := mono.Measure(steps[i])
					pad := float64(mono.Size) * 0.45
					p.RoundRect(x-pad, cy-float64(mono.Size)*0.75, w+2*pad, float64(mono.Size)*1.5, pad, 0, th.Panel, a)
					mono.FX = decker.FadeUp(t, 0.4, mono.Size)
					mono.DrawMid(p, steps[i], x, cy)
					continue
				}
				// Wrapped to the row on a narrow screen, and centered on it.
				txt.FX, txt.MaxW = decker.FadeUp(t, 0.4, txt.Size), row.Right()-x
				_, h := txt.Measure(steps[i])
				txt.Draw(p, steps[i], x, cy-h/2)
			}
		}}
}
