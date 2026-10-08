package main

import (
	"fmt"

	"github.com/austinlparker/decker"
)

func titleSlide() decker.Slide {
	return decker.Slide{Title: "Starter", Section: "Intro",
		Notes: "Replace this deck with your own. Run go run . -review review after every change.",
		View: func(c decker.Ctx, sc *decker.Scene) {
			th := c.Theme
			box := c.Frame().Inset(c.X(0.08), c.Y(0.2))
			title, rest := box.CutTop(box.H * 0.6)
			size, s := th.Display.Fit("Your talk's title", title.W, title.H, c.Size(0.2), 0)
			decker.Text{Font: th.Display, Size: size, Color: th.Accent, FX: decker.RiseIn(c.T, 0.02, size)}.
				Draw(sc.Px, s, title.X, title.Y)
			sub := decker.Text{Font: th.Body, Size: c.SmallText(th.Body), Color: th.Muted, MaxW: rest.W,
				FX: decker.FadeUp(c.T-0.4, 0.4, c.SmallText(th.Body))}
			sub.Draw(sc.Px, "a subtitle, a name, a date", rest.X, rest.Y+c.Unit(0.03))
		}}
}

func pointSlide() decker.Slide {
	return decker.Slide{Title: "One idea per slide", Steps: 2,
		Notes:   "The headline is the point. The second build is the evidence.",
		Sources: []decker.Source{{Label: "Your citation", URL: "https://example.com/source"}},
		View: func(c decker.Ctx, sc *decker.Scene) {
			th := c.Theme
			body := heading(c, sc.Px, "One idea per slide")
			claim, evidence := body.CutTop(body.H * 0.55)
			size, s := th.Display.Fit("Say the point in the headline.", claim.W, claim.H, c.Size(0.12), 0)
			decker.Text{Font: th.Display, Size: size, Color: th.Accent}.Draw(sc.Px, s, claim.X, claim.Y)
			if c.Reached(1) {
				small := c.SmallText(th.Body)
				decker.Text{Font: th.Body, Size: small, Color: th.Text, MaxW: evidence.W,
					FX: decker.FadeUp(c.Since(1), 0.4, small)}.
					Draw(sc.Px, "Put the evidence on the next build, and the detail in the notes.", evidence.X, evidence.Y)
			}
		}}
}

// budget is a latency budget: where 300ms of a request go.
var budget = []struct {
	name string
	ms   float64
}{{"edge", 20}, {"auth", 35}, {"service", 90}, {"database", 120}, {"render", 35}}

// budgetSlide is a visual built for this talk from primitives: one row per
// hop, a bar on a shared millisecond scale, one hop per build, and on the
// last build every hop but the slow one dimmed. It lays out first, from its rect and data,
// checks that layout fits, and only then draws.
func budgetSlide() decker.Slide {
	return decker.Slide{Title: "Where the time goes", Section: "Evidence", Steps: len(budget),
		Notes: "Each build adds one hop. Database is the one to talk about.",
		View: func(c decker.Ctx, sc *decker.Scene) {
			th, p := c.Theme, sc.Px
			body := heading(c, p, "Where the time goes")
			gap := c.Unit(0.02)

			// Layout: a column of names sized to the longest, a column for
			// the times, and the bars between them on one scale.
			text := decker.Text{Font: th.Body, Size: c.SmallText(th.Body), Color: th.Text}
			nameW, rowH, slowest := 0.0, 0.0, 0.0
			for _, b := range budget {
				w, h := text.Measure(b.name)
				nameW, rowH, slowest = max(nameW, w), max(rowH, h), max(slowest, b.ms)
			}
			timeW, _ := text.Measure("000ms")
			c.Fits("budget rows", body, body.W, float64(len(budget))*rowH*1.2)
			names, rest := body.CutLeft(nameW + gap)
			_, bars := rest.CutRight(timeW + gap)
			scale := decker.NiceScale(0, slowest, 4, bars.X, bars.Right())
			rows := names.Rows(0, 1, 1, 1, 1, 1)

			for i, b := range budget {
				if !c.Reached(i) {
					break
				}
				row := rows[i]
				mid := row.Y + row.H/2
				grow := decker.Ease(c.Since(i), 0.5)
				col := th.SeriesColor(i)
				if last := len(budget) - 1; b.name != "database" && c.Reached(last) {
					col = decker.Mix(col, th.Background, 0.65*decker.Ease(c.Since(last), 0.4))
				}
				end := scale.At(b.ms * grow)
				barH := min(row.H*0.6, rowH)
				p.RoundRect(bars.X, mid-barH/2, max(end-bars.X, 1), barH, barH/4, 0, col, 1)
				fade := decker.FadeUp(c.Since(i)-0.2, 0.3, text.Size)
				name := text
				name.Align, name.Color = decker.Right, th.Muted
				name.DrawMid(p, b.name, names.Right()-gap, mid)
				ms := text
				ms.FX = fade
				ms.DrawMid(p, fmt.Sprintf("%.0fms", b.ms), end+gap/2, mid)
			}
		}}
}
