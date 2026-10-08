// Command showcase is the deck behind the README's example videos: three short
// clips of what a talk can draw. Each clip is a range of slides; record.sh
// exports them with -video and turns them into GIFs.
package main

import (
	"strings"

	"github.com/austinlparker/decker"
)

func main() { decker.Main(talk()) }

var theme = &decker.Theme{
	Background: decker.Hex("#0B1020"),
	Text:       decker.Hex("#EEF2F8"),
	Muted:      decker.Hex("#94A0B8"),
	Faint:      decker.Hex("#2A3350"),
	Accent:     decker.Hex("#5AD2FF"),
	Accent2:    decker.Hex("#FF8A3D"),
	Warn:       decker.Hex("#FF5C7A"),
	Good:       decker.Hex("#7BE07B"),
	Panel:      decker.Hex("#121A30"),
	Display:    decker.StockFont("SpaceGrotesk-Bold"),
	Body:       decker.StockFont("SpaceGrotesk-Medium"),
	Mono:       decker.StockFont("JetBrainsMono-ExtraBold"),
	Overlay: func(c decker.Ctx, p *decker.Pixels) {
		th := c.Theme
		decker.ProgressBar(c, p, decker.NewRect(0, c.PH()-c.Unit(0.008), c.PW(), c.Unit(0.008)), th.Accent, th.Faint)
		if c.Section != "" {
			decker.Label(c, p, strings.ToUpper(c.Section), c.X(0.04), c.Y(0.9), th.Muted, decker.Left)
		}
		decker.PageNumber(c, p, c.X(0.96), c.Y(0.9), decker.Right, th.Muted)
	},
}

var stencil = decker.StockFigletFont("Decker Stencil")

func talk() decker.Deck {
	return decker.Deck{Name: "decker-showcase", Theme: theme, Slides: []decker.Slide{
		// Clip 1, slides 1-3: type and letter effects.
		titleSlide(),
		statementSlide(),
		effectsSlide(),
		// Clip 2, slides 4-5: charts and numbers.
		statsSlide(),
		chartsSlide(),
		// Clip 3, slides 6-8: code, diagrams and magic move.
		codeSlide(),
		pipelineSlide(),
		pipelineMovedSlide(),
	}}
}

// heading draws a slide's title and returns the y below it.
func heading(c decker.Ctx, p *decker.Pixels, s string) float64 {
	th := c.Theme
	t := decker.Text{Font: th.Display, Size: c.Size(0.11), Color: th.Text}.Fit(s, c.X(0.88), c.Y(0.14))
	t.FX = decker.RiseIn(c.T, 0.02, t.Size)
	_, h := t.Draw(p, s, c.X(0.06), c.Y(0.07))
	p.Rect(c.X(0.06), c.Y(0.07)+h+c.Unit(0.015), c.X(0.06)*decker.EaseOutExpo(decker.Progress(c.T, 0.1, 0.7)), c.Unit(0.008), th.Accent, 1)
	return c.Y(0.07) + h + c.Unit(0.05)
}

func titleSlide() decker.Slide {
	return decker.Slide{Title: "Decker", Section: "Type", Hold: 3.5,
		View: func(c decker.Ctx, sc *decker.Scene) {
			p, th := sc.Px, c.Theme
			p.RadialGradient(0, 0, c.PW(), c.PH(), 0, c.X(0.5), c.Y(0.42), c.X(0.6), th.Faint, th.Background, 1)
			f, lines, scale := decker.FitBlock("DECKER", c.X(0.84), c.Y(0.42), 1, 0, stencil)
			_, h := decker.Block{Font: f, Scale: scale, Color: th.Accent, To: &th.Accent2, Align: decker.Center,
				Glow: 0.6, FX: decker.BlockDecrypt(c.T, 1.4, th.Faint)}.
				Draw(p, strings.Join(lines, "\n"), c.X(0.5), c.Y(0.18))
			size := c.Size(0.075)
			decker.Text{Font: th.Body, Size: size, Color: th.Muted, Align: decker.Center,
				FX: decker.Decode(c.T-1.2, 0.9, 3)}.
				Draw(p, "slide decks as Go programs", c.X(0.5), c.Y(0.18)+h+c.Unit(0.06))
		}}
}

func statementSlide() decker.Slide {
	return decker.Slide{Title: "Statement", Steps: 2, Hold: 2.2, Transition: decker.TransitionGlitch,
		View: func(c decker.Ctx, sc *decker.Scene) {
			p, th := sc.Px, c.Theme
			box := c.Rect(0.08, 0.2, 0.84, 0.36)
			spans := decker.ParseSpans("Every slide is a *function* of `time`.", th)
			r := decker.Rich{Font: th.Body, Size: c.Size(0.16), Color: th.Text}.Fit(spans, box.W, box.H)
			r.FX = decker.RiseIn(c.T, 0.025, r.Size)
			r.Draw(p, spans, box.X, box.Y)
			if c.Reached(1) {
				size := c.Size(0.08)
				decker.Text{Font: th.Display, Size: size, Color: th.Accent2, Glow: 0.5,
					Shine: decker.ShineBand(c.Since(1)-0.5, 1.2, 0.8),
					FX:    decker.TypeOn(c.Since(1), 28)}.
					Draw(p, "Replay it, snapshot it, render it to video.", c.X(0.08), c.Y(0.66))
			}
		}}
}

func effectsSlide() decker.Slide {
	return decker.Slide{Title: "Letter effects", Hold: 3.5, Transition: decker.TransitionIris,
		View: func(c decker.Ctx, sc *decker.Scene) {
			p, th := sc.Px, c.Theme
			top := heading(c, p, "Letter effects")
			rows := decker.NewRect(c.X(0.06), top, c.X(0.88), c.Y(0.88)-top).Rows(c.Unit(0.03), 1, 1, 1)
			fx := []struct {
				word string
				col  decker.RGB
				fx   func(size int) decker.GlyphEffect
			}{
				{"Wave", th.Accent, func(size int) decker.GlyphEffect {
					return decker.Chain(decker.RiseIn(c.T, 0.04, size), decker.Wave(c.T, float64(size)*0.12))
				}},
				{"Decode", th.Good, func(int) decker.GlyphEffect { return decker.Decode(c.T-0.3, 1.2, 7) }},
				{"DropIn", th.Accent2, func(size int) decker.GlyphEffect { return decker.DropIn(c.T-0.6, 0.06, size) }},
			}
			for i, f := range fx {
				r := rows[i]
				t := decker.Text{Font: th.Display, Size: c.Size(0.2), Color: f.col, Glow: 0.3}.Fit(f.word, r.W*0.45, r.H)
				t.FX = f.fx(t.Size)
				t.DrawMid(p, f.word, r.X, r.Y+r.H/2)
			}
			blk := decker.NewRect(c.X(0.52), top, c.X(0.42), c.Y(0.88)-top)
			f, lines, scale := decker.FitBlock("BLOCK FX", blk.W, blk.H, 2, 1, decker.BlockShadow)
			decker.Block{Font: f, Scale: scale, Color: th.Warn, To: &th.Accent2, Gap: 1, Align: decker.Right,
				FX: decker.BlockChain(decker.BlockRain(c.T-0.2), decker.BlockBeam(c.T-1.2, 1.2))}.
				Draw(p, strings.Join(lines, "\n"), blk.Right(), blk.Y+blk.H*0.15)
		}}
}

func statsSlide() decker.Slide {
	return decker.Slide{Title: "Numbers", Section: "Data", Steps: 2, Hold: 2.6, Transition: decker.TransitionPush,
		View: func(c decker.Ctx, sc *decker.Scene) {
			p, th := sc.Px, c.Theme
			top := heading(c, p, "Numbers that count up")
			_, page := c.Frame().Inset(c.X(0.06), c.Y(0.1)).CutTop(top - c.Y(0.1))
			cols := page.Cols(c.Unit(0.06), 1, 1, 1)
			stats := []decker.Stat{
				{Value: 99.97, Suffix: "%", Decimals: 2, Label: "uptime"},
				{Value: 42, Suffix: " ms", Label: "p99 latency"},
				{Value: 1.8, Prefix: "$", Suffix: "M", Decimals: 1, Label: "saved per year", Step: 1},
			}
			for i, s := range stats {
				fx := decker.AppearAt(c, s.Step, -1, func(t float64) decker.Composite {
					return decker.FlyIn(c, t-0.12*float64(i), 0.5, decker.DirDown, 0.08)
				}, nil)
				r := cols[i].Sub(0, 0.2, 1, 0.6)
				fx.Draw(p, r, func(p *decker.Pixels) {
					p.RoundRect(r.X, r.Y, r.W, r.H, c.Unit(0.03), 0, th.Panel, 1)
					s.Draw(c, p, r.Inset(c.Unit(0.04), c.Unit(0.05)))
				})
			}
		}}
}

func chartsSlide() decker.Slide {
	return decker.Slide{Title: "Charts", Steps: 2, Hold: 2.6, Transition: decker.TransitionWipe,
		View: func(c decker.Ctx, sc *decker.Scene) {
			p := sc.Px
			top := heading(c, p, "Charts that build")
			_, page := c.Frame().Inset(c.X(0.06), c.Y(0.1)).CutTop(top - c.Y(0.1))
			cols := page.Cols(c.Unit(0.08), 3, 2)
			decker.LineChart{
				Labels: []string{"Jan", "Feb", "Mar", "Apr", "May", "Jun"},
				Series: [][]float64{{12, 19, 17, 28, 34, 45}, {8, 11, 15, 14, 20, 26}},
				Names:  []string{"requests", "errors ×10"},
				Points: true,
			}.Draw(c, p, cols[0])
			decker.DonutChart{
				Labels: []string{"Go", "Rust", "Zig"},
				Values: []float64{62, 27, 11},
				Center: "62%",
				Step:   1,
			}.Draw(c, p, cols[1])
		}}
}

const codeSrc = `func view(c Ctx, sc *Scene) {
	t := Text{Font: f, Size: 48}
	t.FX = RiseIn(c.T, 0.02, 48)
	t.Draw(sc.Px, "Hi", 0, 0)
}`

func codeSlide() decker.Slide {
	return decker.Slide{Title: "Code", Section: "Code & diagrams", Steps: 3, Hold: 1.8, Transition: decker.TransitionZoom,
		View: func(c decker.Ctx, sc *decker.Scene) {
			p := sc.Px
			top := heading(c, p, "Code that walks through itself")
			decker.Code{
				Source: codeSrc, Lang: "go", LineNumbers: true, Title: "view.go",
				Focus:     []decker.LineRange{{From: 2, To: 3}, {From: 4, To: 4}},
				FirstStep: 1,
			}.Draw(c, p, decker.NewRect(c.X(0.06), top, c.X(0.88), c.Y(0.88)-top))
		}}
}

// pipeline draws three placed panels joined by connectors; moved lays them
// out as a column, and TransitionMorph glides each panel between the layouts.
func pipeline(c decker.Ctx, sc *decker.Scene, moved bool) {
	p, th := sc.Px, c.Theme
	title := "Diagrams that draw on"
	if moved {
		title = "…and magic move"
	}
	top := heading(c, p, title)
	area := decker.NewRect(c.X(0.06), top+c.Unit(0.02), c.X(0.88), c.Y(0.86)-top)
	names := []string{"Ctx", "View", "Scene"}
	cols := []decker.RGB{th.Accent, th.Accent2, th.Good}
	var boxes []decker.Rect
	if moved {
		for _, r := range area.Sub(0.55, 0, 0.45, 1).Rows(c.Unit(0.05), 1, 1, 1) {
			boxes = append(boxes, r)
		}
	} else {
		for _, r := range area.Cols(c.Unit(0.12), 1, 1, 1) {
			boxes = append(boxes, r.Sub(0, 0.25, 1, 0.45))
		}
	}
	for i := range boxes {
		if i > 0 {
			step := 0
			if !moved {
				step = i
			}
			decker.Connector{From: boxes[i-1], To: boxes[i], Route: decker.RouteCurved, Head: decker.HeadArrow,
				Color: th.Muted, Prog: decker.Ease(c.Since(step)-0.2, 0.6)}.Draw(c, p)
		}
	}
	for i, r := range boxes {
		name, col := names[i], cols[i]
		if !moved && !c.Reached(i) {
			continue
		}
		sc.Place(name, r, func(p *decker.Pixels, r decker.Rect) {
			decker.Panel(c, p, r.X, r.Y, r.W, r.H, name, th.Panel, col, th.Text, 1)
		})
	}
	if moved {
		side := area.Sub(0, 0.1, 0.48, 0.8)
		decker.BulletList(c, p, []string{
			"Place it by key",
			"Place it again",
			"Watch it glide",
		}, side.X, side.Y, side.W, side.H, 0)
	}
}

func pipelineSlide() decker.Slide {
	return decker.Slide{Title: "Diagram", Steps: 3, Hold: 1.4, Transition: decker.TransitionFadeThrough,
		View: func(c decker.Ctx, sc *decker.Scene) { pipeline(c, sc, false) }}
}

func pipelineMovedSlide() decker.Slide {
	return decker.Slide{Title: "Magic move", Steps: 3, Hold: 1.3, Transition: decker.TransitionMorph.Over(1.1),
		View: func(c decker.Ctx, sc *decker.Scene) { pipeline(c, sc, true) }}
}
