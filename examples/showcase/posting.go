package main

import "github.com/austinlparker/decker"

// The posting section: slides whose posts the rest of the deck can't show
// (notes.md has the posts).

// buildsSlide reveals a panel a build. Its post attaches an image of each
// build, and six go up as a gallery.
func buildsSlide() decker.Slide {
	return decker.Slide{Title: "Six builds", Section: "Posting", Steps: 6, Hold: 0.8, Transition: decker.TransitionCover,
		View: func(c decker.Ctx, sc *decker.Scene) {
			p, th := sc.Px, c.Theme
			top := heading(c, p, "Every build, an image")
			area := decker.NewRect(c.X(0.06), top, c.X(0.88), c.Y(0.86)-top)
			names := []string{"say it", "move on", "render", "upload", "post", "reply"}
			cols := []decker.RGB{th.Accent, th.Accent2, th.Good, th.Accent, th.Accent2, th.Good}
			for i, r := range area.Grid(3, 2, c.Unit(0.03)) {
				if !c.Reached(i) {
					continue
				}
				a := decker.Ease(c.Since(i), 0.35)
				decker.Panel(c, p, r.X, r.Y, r.W, r.H, names[i], th.Panel, cols[i], th.Text, a)
			}
		}}
}

// grainSlide sets every pixel itself: a gradient under film grain, from
// Hash01 so each frame is the same. That's detail a PNG can't squeeze under
// Bluesky's 2MB but a JPEG can, so the slide's image goes up as a JPEG.
func grainSlide() decker.Slide {
	return decker.Slide{Title: "Film grain", Hold: 2, Transition: decker.TransitionPixelate,
		View: func(c decker.Ctx, sc *decker.Scene) {
			p := sc.Px
			top, bottom, edge := decker.Hex("#1B2A6B"), decker.Hex("#E0703A"), c.Theme.Background
			clamp := func(v float32) float32 { return min(max(v, 0), 255) }
			for y := range p.H {
				row := decker.Mix(top, bottom, float64(y)/float64(p.H))
				for x := range p.W {
					col := decker.Mix(row, edge, 0.5*float64(x)/float64(p.W))
					g := float32(decker.Hash01(x, y, 7)-0.5) * 20
					p.Pix[y*p.W+x] = decker.RGB{R: clamp(col.R + g), G: clamp(col.G + g), B: clamp(col.B + g)}
				}
			}
			heading(c, p, "Every pixel, by hand")
		}}
}

func skipSlide() decker.Slide {
	return decker.Slide{Title: "Skip me", Hold: 1.5, Transition: decker.TransitionSplit,
		View: func(c decker.Ctx, sc *decker.Scene) {
			p := sc.Px
			top := heading(c, p, "Skip me")
			decker.BulletList(c, p, []string{"Jump over a slide and it doesn't post", "Come back, move on, and it posts at the end"},
				c.X(0.06), top, c.X(0.88), c.Y(0.86)-top, 0)
		}}
}

func thanksSlide() decker.Slide {
	return decker.Slide{Title: "Thanks", Hold: 2.5, Transition: decker.TransitionFade,
		View: func(c decker.Ctx, sc *decker.Scene) {
			p, th := sc.Px, c.Theme
			t := decker.Text{Font: th.Display, Size: c.Size(0.2), Color: th.Accent, Align: decker.Center, Glow: 0.4}.
				Fit("Thanks", c.X(0.8), c.Y(0.3))
			t.FX = decker.RiseIn(c.T, 0.03, t.Size)
			t.Draw(p, "Thanks", c.X(0.5), c.Y(0.3))
			size := c.SmallText(th.Body)
			decker.Text{Font: th.Body, Size: size, Color: th.Muted, Align: decker.Center, FX: decker.FadeUp(c.T-0.5, 0.4, size)}.
				Draw(p, "the last slide posts as soon as it's up", c.X(0.5), c.Y(0.66))
		}}
}
