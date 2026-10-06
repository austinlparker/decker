package decker_test

// The gallery is a deck whose slides together exercise every exported
// drawing API of the engine. Its golden hashes (testdata/gallery.golden)
// pin the engine's output byte for byte: a refactor that moves a pixel, a
// cell or an escape sequence fails TestGalleryGolden. Record with
// UPDATE_GOLDEN=1 go test -run TestGalleryGolden, and only when a change in
// output is intended.
//
// Every View is a pure function of its Ctx: no clock, no rand. Animated
// things vary with c.T / c.StepT so the golden moments differ.

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"

	. "github.com/austinlparker/decker"
	"github.com/austinlparker/decker/decktest"
)

func TestGalleryGolden(t *testing.T) {
	decktest.Golden(t, gallery(), "testdata/gallery.golden")
}

func TestGallerySlides(t *testing.T) { decktest.Slides(t, gallery()) }

// decktest.Hashes and decktest.Hash are API: talks diff them in CI.
var (
	_ = decktest.Hashes
	_ decktest.Hash
)

// TestGalleryDraw covers Deck.Draw and Deck.Steps, which the golden does
// not (it goes through Render).
func TestGalleryDraw(t *testing.T) {
	d := gallery()
	wantSteps := map[string]int{"Cycle": 5, "Cycle ring": 4, "Bullets": 5, "Builds": 4}
	for i, s := range d.Slides {
		if want, ok := wantSteps[s.Title]; ok && d.Steps(i) != want {
			t.Errorf("slide %q has %d steps, want %d", s.Title, d.Steps(i), want)
		}
		for step := 0; step < d.Steps(i); step++ {
			d.Draw(i, Ctx{W: 100, H: 30, T: 1, Step: step, StepT: 1, Theme: d.Theme})
		}
	}
}

// TestGalleryPanics pins the panics the engine promises for misuse.
func TestGalleryPanics(t *testing.T) {
	th := galleryTheme()
	for name, tc := range map[string]struct {
		f    func()
		want string
	}{
		"text without a font": {func() { Text{Size: 10}.Draw(NewPixels(10, 10, th.Background), "x", 0, 0) }, "Font is required"},
		"missing font file":   {func() { LoadFont(fstest.MapFS{}, "nope.ttf") }, "nope.ttf"},
		"broken font":         {func() { LoadFont(fstest.MapFS{"x.ttf": {Data: []byte("junk")}}, "x.ttf") }, "x.ttf"},
		"missing fig font":    {func() { LoadFigFont(fstest.MapFS{}, "nope.flf") }, "nope.flf"},
	} {
		func() {
			defer func() {
				r := recover()
				if r == nil || !strings.Contains(fmt.Sprint(r), tc.want) {
					t.Errorf("%s: panic = %v, want one mentioning %q", name, r, tc.want)
				}
			}()
			tc.f()
		}()
	}
	if _, err := ParseFont([]byte("junk")); err == nil {
		t.Error("ParseFont accepted junk")
	}
}

// ---- the theme and the deck ----

func galleryTheme() *Theme {
	body := StockFont("SpaceGrotesk-Medium")
	// A bolder cut below 14px, to exercise Font.Small.
	body.Small, body.SmallBelow = StockFont("SpaceGrotesk-Bold"), 14
	return &Theme{
		Background: Hex("#101820"),
		Text:       Hex("#F0F0F0"),
		Muted:      Hex("#A0A8B0"),
		Faint:      Hex("#384048"),
		Accent:     Hex("#FF7A00"),
		Accent2:    Hex("#40C0FF"),
		Warn:       Hex("#FF6060"),
		Good:       Hex("#70D050"),
		Panel:      Hex("#0A1016"),
		Display:    StockFont("SpaceGrotesk-Bold"),
		Body:       body,
		Mono:       StockFont("JetBrainsMono-ExtraBold"),
		Overlay: func(c Ctx, p *Pixels) {
			p.Disc(float64(p.W)-4, float64(p.H)-4, 2+Pulse(c.T, 2), Hex("#FF7A00"), 0.9)
			p.Glow(float64(p.W)-4, float64(p.H)-4, 6, Hex("#FF7A00"), 0.3)
		},
	}
}

func gallery() Deck {
	th := galleryTheme()
	ims := NewImages(galleryImages(), "img")
	return Deck{Name: "gallery", Theme: th, Slides: []Slide{
		slideText(),
		slideMetrics(),
		slideLetterFX(),
		slideBlockStockA(),
		slideBlockStockB(),
		slideBlockOptions(),
		slideBlockFXA(),
		slideBlockFXB(),
		slideFigFont(),
		slideComponents(),
		slideCycle(),
		slideCycleRing(),
		slideBullets(),
		slidePixels(),
		slidePixelArt(),
		slideImages(ims),
		slideMotion(),
		slideScene(),
		slideBuilds(),
		slideLayout(),
		slideMorph(false),
		slideMorph(true),
		slideImageAlpha(ims),
		slidePosition(),
	}}
}

// ---- helpers ----

// heading draws a slide title top-left and returns the y below it.
func heading(c Ctx, p *Pixels, s string) float64 {
	size := max(c.Size(0.06), c.SmallText(c.Theme.Display))
	_, h := Text{Font: c.Theme.Display, Size: size, Color: c.Theme.Accent}.Draw(p, s, c.X(0.03), c.Y(0.02))
	return c.Y(0.02) + h
}

// tag draws a small mono caption.
func tag(c Ctx, p *Pixels, s string, x, y float64, col RGB) (float64, float64) {
	return Text{Font: c.Theme.Mono, Size: c.SmallText(c.Theme.Mono), Color: col}.Draw(p, s, x, y)
}

func rowY(c Ctx, top float64, n, i int) float64 {
	return top + (c.Y(0.97)-top)*float64(i)/float64(n)
}

func rgbp(c RGB) *RGB { return &c }

// galleryImages is a tiny in-memory image directory: a gradient, a wide
// banner and a translucent ramp, generated so no binary file is needed.
func galleryImages() fstest.MapFS {
	enc := func(img image.Image) *fstest.MapFile {
		var b bytes.Buffer
		if err := png.Encode(&b, img); err != nil {
			panic(err)
		}
		return &fstest.MapFile{Data: b.Bytes()}
	}
	grad := image.NewRGBA(image.Rect(0, 0, 64, 40))
	for y := 0; y < 40; y++ {
		for x := 0; x < 64; x++ {
			b := uint8(60)
			if (x/8+y/8)%2 == 0 {
				b = 200
			}
			grad.Set(x, y, color.RGBA{uint8(x * 255 / 63), uint8(y * 255 / 39), b, 255})
		}
	}
	wide := image.NewRGBA(image.Rect(0, 0, 400, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 400; x++ {
			v := uint8(128 + 127*math.Sin(float64(x)/9)*math.Cos(float64(y)/7))
			wide.Set(x, y, color.RGBA{v, uint8(x * 255 / 399), 255 - v, 255})
		}
	}
	ramp := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			ramp.Set(x, y, color.NRGBA{255, uint8(y * 8), 40, uint8(x * 255 / 31)})
		}
	}
	return fstest.MapFS{
		"img/logo.png": enc(logoImage(48)),
		"img/grad.png": enc(grad),
		"img/wide.png": enc(wide),
		"img/ramp.png": enc(ramp),
	}
}

// logoImage is a transparent-background mark: an opaque disc with a
// half-transparent ring around it, the shape a logo PNG usually has.
func logoImage(n int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, n, n))
	h := float64(n) / 2
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			d := math.Hypot(float64(x)+0.5-h, float64(y)+0.5-h) / h
			switch {
			case d < 0.5:
				img.SetNRGBA(x, y, color.NRGBA{255, 120, 0, 255})
			case d < 0.9:
				img.SetNRGBA(x, y, color.NRGBA{255, 255, 255, 128})
			}
		}
	}
	return img
}

// ---- slides ----

func slideText() Slide {
	return Slide{Title: "Text fields", Notes: "every Text field", Transition: TransitionDefault,
		View: func(c Ctx, sc *Scene) {
			p, th := sc.Px, c.Theme
			top := heading(c, p, "Text fields")
			s := max(c.Size(0.075), c.Size(MinText))

			// Alignment against a guide line.
			gx := c.X(0.5)
			p.Rect(gx, top, 1, c.Y(0.3), th.Faint, 1)
			for i, a := range []Align{Left, Center, Right} {
				w, h := Text{Font: th.Display, Size: s, Color: th.Text, Align: a}.Draw(p, [...]string{"Left", "Center", "Right"}[i], gx, top+float64(i)*float64(s)*DefaultLeading)
				p.Rect(gx, top+float64(i)*float64(s)*DefaultLeading+h, w, 1, th.Accent2, 0.6)
			}

			// Gradient, glow with and without a color, shine.
			y := c.Y(0.40)
			Text{Font: th.Display, Size: s, Color: th.Accent, To: rgbp(th.Accent2)}.Draw(p, "Gradient", c.X(0.03), y)
			Text{Font: th.Display, Size: s, Color: th.Text, Glow: 1.2, GlowColor: rgbp(th.Warn)}.Draw(p, "Warm glow", c.X(0.28), y)
			Text{Font: th.Display, Size: s, Color: th.Accent2, Glow: 0.6}.Draw(p, "Glow", c.X(0.58), y)
			Text{Font: th.Display, Size: s, Color: th.Muted, Shine: ShineBand(c.T, 2, 0.9)}.Draw(p, "Shine", c.X(0.75), y)

			// Wrapping, leading and the two other faces.
			y = c.Y(0.56)
			bs := max(c.Size(0.05), c.SmallText(th.Body))
			w, h := Text{Font: th.Body, Size: bs, Color: th.Text, MaxW: c.X(0.3), Leading: 1.4}.
				Draw(p, "A longer paragraph that has to wrap inside its box, then a hard\nbreak.", c.X(0.03), y)
			p.Rect(c.X(0.03), y+h, w, 1, th.Faint, 1)
			Text{Font: th.Body, Size: bs, Color: th.Text, MaxW: c.X(0.3), Align: Center}.
				Draw(p, "Centered and wrapped inside a narrow column of text", c.X(0.5), y)
			Text{Font: th.Mono, Size: bs, Color: th.Good, Align: Right, MaxW: c.X(0.25)}.
				Draw(p, "mono: func main() {}", c.X(0.97), y)

			// Per-glyph FX and DrawMid, and Baseline.
			y = c.Y(0.82)
			t := Text{Font: th.Display, Size: s, Color: th.Text, FX: RiseIn(c.T, 0.05, s)}
			tw, _ := t.Draw(p, "Per glyph", c.X(0.03), y)
			p.Rect(c.X(0.03), y+t.Baseline(), tw, 1, th.Warn, 0.8)
			box := c.Unit(0.12)
			p.RoundRect(c.X(0.4), y, c.X(0.2), box, 3, 1, th.Faint, 1)
			Text{Font: th.Body, Size: bs, Color: th.Text, Align: Center}.DrawMid(p, "Mid gjpqy", c.X(0.5), y+box/2)
			Text{Font: th.Display, Size: s, Color: th.Accent, Align: Right}.DrawMid(p, "DrawMid", c.X(0.97), y+box/2)
			Text{Font: th.Body, Size: bs, Color: th.Text}.DrawMid(p, "   ", c.X(0.62), y+box/2) // no ink: centered on the cap height
			Text{Font: th.Body, Size: bs, Color: th.Muted}.DrawMid(p, "-", c.X(0.65), y+box/2)
		}}
}

func slideMetrics() Slide {
	return Slide{Title: "Font metrics", Transition: TransitionNone,
		View: func(c Ctx, sc *Scene) {
			p, th := sc.Px, c.Theme
			top := heading(c, p, "Font metrics")
			f := th.Display

			// Fit: biggest size that fits a box, and the wrapped text.
			bx, by, bw, bh := c.X(0.03), top, c.X(0.3), c.Y(0.22)
			p.RoundRect(bx, by, bw, bh, 2, 1, th.Faint, 1)
			size, wrapped := f.Fit("What Your MCP Server Does", bw, bh, c.Size(0.3), 0)
			Text{Font: f, Size: size, Color: th.Text}.Draw(p, wrapped, bx, by)
			size2, wrapped2 := f.Fit("Tight leading wraps here too", bw, bh, c.Size(0.3), 0.95)
			p.RoundRect(bx, by+bh+c.Unit(0.03), bw, bh, 2, 1, th.Faint, 1)
			Text{Font: f, Size: size2, Color: th.Muted, Leading: 0.95}.Draw(p, wrapped2, bx, by+bh+c.Unit(0.03))

			// FitAll: one size for several parts.
			ax := c.X(0.38)
			p.RoundRect(ax, by, bw, bh, 2, 1, th.Faint, 1)
			fs, parts := FitAll(th.Body, []string{"short", "a medium length line", "the longest line of the three goes here"}, bw, bh, c.Size(0.2))
			yy := by
			for _, part := range parts {
				_, h := Text{Font: th.Body, Size: fs, Color: th.Accent2}.Draw(p, part, ax, yy)
				yy += h + 2
			}

			// Wrap, Measure, Ascent, CapHeight, Ink, Drawn.
			rx := c.X(0.72)
			ms := max(c.Size(0.07), c.SmallText(f))
			for i, l := range th.Body.Wrap("Balanced wrapping keeps a heading even", ms, c.X(0.2)) {
				Text{Font: th.Body, Size: ms, Color: th.Text}.Draw(p, l, rx, top+float64(i)*float64(ms)*DefaultLeading)
			}
			y := c.Y(0.62)
			word := "Hamburgefonstiv gjpqy"
			base := y + f.Ascent(ms)
			p.Rect(c.X(0.03), base, c.X(0.6), 1, th.Warn, 1)
			p.Rect(c.X(0.03), base-f.CapHeight(ms), c.X(0.6), 1, th.Good, 1)
			Text{Font: f, Size: ms, Color: th.Text}.Draw(p, word, c.X(0.03), y)
			w := f.Measure(word, ms)
			p.Rect(c.X(0.03), base+f.Ascent(ms)*0.4, w, 2, th.Accent, 1)
			ink0, ink1 := f.Ink(word, ms)
			p.Rect(c.X(0.03)+w+4, base+ink0, 3, ink1-ink0, th.Accent2, 1)
			e0, e1 := f.Ink("   ", ms)
			p.Rect(c.X(0.03)+w+12, base+e0, 3, e1-e0+2, th.Muted, 1)

			// The Small cut: a size below SmallBelow, and the sizes actually drawn.
			y = c.Y(0.75)
			x := c.X(0.03)
			for _, sz := range []int{8, 10, 12, 13, 14, 16, 24} {
				d := th.Body.Drawn(sz)
				w, _ := Text{Font: th.Body, Size: sz, Color: th.Text}.Draw(p, fmt.Sprintf("%dpx", sz), x, y)
				p.Rect(x, y+float64(d)*1.2, w, 1, th.Faint, 1)
				x += w + c.Unit(0.05)
			}
			Label(c, p, fmt.Sprintf("drawn %d %d", th.Body.Drawn(c.Size(0.05)), th.Display.Drawn(c.Size(0.05))), c.X(0.03), c.Y(0.9), th.Muted, Left)
		}}
}

func slideLetterFX() Slide {
	return Slide{Title: "Letter effects", Transition: TransitionPush,
		View: func(c Ctx, sc *Scene) {
			p, th := sc.Px, c.Theme
			top := heading(c, p, "Letter effects")
			s := max(c.Size(0.06), c.SmallText(th.Display))
			word := "Hello, decker"
			n := len([]rune(word))
			fx := []struct {
				name string
				fx   func(int) GlyphFX
			}{
				{"RiseIn", RiseIn(c.T, 0.06, s)},
				{"DropIn", DropIn(c.T-0.2, 0.05, s)},
				{"Decode", Decode(c.T, 1.5, n)},
				{"TypeOn", TypeOn(c.T, 8)},
				{"FadeUp", FadeUp(c.T, 0.8, s)},
				{"Wave", Wave(c.T, float64(s)*0.15)},
				{"Jitter", Jitter(c.T, 2)},
				{"Chain", Chain(RiseIn(c.T, 0.04, s), Wave(c.T, float64(s)*0.1))},
				{"Chain+Decode", Chain(Decode(c.T-0.3, 1.2, n), Jitter(c.T, 1), FadeUp(c.T, 0.5, s))},
			}
			for i, e := range fx {
				x, row := c.X(0.03), i
				if i >= 5 {
					x, row = c.X(0.53), i-5
				}
				y := rowY(c, top+c.Y(0.03), 5, row)
				tag(c, p, e.name, x, y, th.Muted)
				Text{Font: th.Display, Size: s, Color: th.Text, FX: e.fx}.Draw(p, word, x, y+float64(c.SmallText(th.Mono))*1.2)
			}
			// ShineBand as a Text.Shine, with a gradient under it.
			y := rowY(c, top+c.Y(0.03), 5, 4)
			x := c.X(0.53)
			tag(c, p, "ShineBand", x, y, th.Muted)
			Text{Font: th.Display, Size: s, Color: th.Accent, To: rgbp(th.Accent2), Shine: ShineBand(c.T-0.2, 1.5, 1)}.
				Draw(p, word, x, y+float64(c.SmallText(th.Mono))*1.2)
		}}
}

func slideBlockStockA() Slide {
	return Slide{Title: "Block stock A", Transition: TransitionDissolve,
		View: func(c Ctx, sc *Scene) {
			p, th := sc.Px, c.Theme
			top := heading(c, p, "Block stock A")
			for i, f := range []*FigFont{BlockShadow, BlockSolid, BlockSmall} {
				y := rowY(c, top, 3, i)
				ff, lines, scale := FitBlock("Block "+f.Name[:3], c.X(0.8), (c.Y(0.97)-top)/3-c.Unit(0.02), 1, 0, f)
				tag(c, p, fmt.Sprintf("%s rows=%d scale=%.1f", ff.Name, ff.Rows(), scale), c.X(0.03), y, th.Muted)
				Block{Font: ff, Scale: scale, Color: th.Accent, FX: BlockFade(c.T, 0.8)}.
					Draw(p, strings.Join(lines, "\n"), c.X(0.03), y+float64(c.SmallText(th.Mono))*1.2)
			}
		}}
}

func slideBlockStockB() Slide {
	return Slide{Title: "Block stock B", Transition: TransitionWipe,
		View: func(c Ctx, sc *Scene) {
			p, th := sc.Px, c.Theme
			top := heading(c, p, "Block stock B")
			for i, f := range []*FigFont{BlockHuge, BlockFancy} {
				y := rowY(c, top, 2, i)
				ff, lines, scale := FitBlock("Deck "+f.Name[:3], c.X(0.8), (c.Y(0.97)-top)/2-c.Unit(0.03), 1, 0, f)
				tag(c, p, fmt.Sprintf("%s rows=%d scale=%.1f", ff.Name, ff.Rows(), scale), c.X(0.03), y, th.Muted)
				Block{Font: ff, Scale: scale, Color: th.Accent2, FX: BlockRain(c.T)}.
					Draw(p, strings.Join(lines, "\n"), c.X(0.03), y+float64(c.SmallText(th.Mono))*1.2)
			}
		}}
}

func slideBlockOptions() Slide {
	return Slide{Title: "Block options", Transition: TransitionPush,
		View: func(c Ctx, sc *Scene) {
			p, th := sc.Px, c.Theme
			top := heading(c, p, "Block options")
			// Fractional and whole scales, below and above 1.
			x := c.X(0.03)
			for _, k := range []float64{0.5, 0.75, 1, 1.5, 2.25} {
				w, _ := Block{Font: BlockSmall, Scale: k, Color: th.Text}.Draw(p, "Ab", x, top)
				x += w + c.Unit(0.03)
			}
			// Color + To gradient, Shadow, Drop, Glow.
			y := top + c.Y(0.14)
			Block{Font: BlockShadow, Scale: 1.5, Color: th.Accent, To: rgbp(th.Accent2), Shadow: th.Faint, Drop: th.Panel, Glow: 0.8}.
				Draw(p, "Dec", c.X(0.03), y)
			Block{Font: BlockSolid, Scale: 1.25, Color: th.Good, Drop: th.Faint}.Draw(p, "Drop", c.X(0.4), y)
			Block{Font: BlockSolid, Scale: 1.25, Color: th.Warn, Glow: 1.4}.Draw(p, "Glow", c.X(0.7), y)
			// Align, Gap and multi-line, with Cells/Size drawn as frames.
			y = c.Y(0.55)
			for i, a := range []Align{Left, Center, Right} {
				ax := [...]float64{c.X(0.03), c.X(0.5), c.X(0.97)}[i]
				b := Block{Font: BlockSmall, Scale: 1, Color: th.Text, Align: a, Gap: 1 + i}
				w, h := b.Draw(p, "Top\nmiddle line\nend", ax, y)
				cw, ch := b.Cells("Top\nmiddle line\nend")
				sw, sh := b.Size("Top\nmiddle line\nend")
				left := ax
				switch a {
				case Center:
					left = ax - w/2
				case Right:
					left = ax - w
				}
				p.RoundRect(left, y, sw, sh, 1, 1, th.Faint, 1)
				tag(c, p, fmt.Sprintf("%dx%d %.0fx%.0f %.0fx%.0f", cw, ch, w, h, sw, sh), left, y+h+c.Unit(0.02), th.Muted)
			}
		}}
}

// blockFXRows draws one row per effect, labelled.
func blockFXRows(c Ctx, p *Pixels, top float64, rows []struct {
	name string
	fx   func(BlockCell) BlockFX
}) {
	n := len(rows)
	for i, r := range rows {
		y := rowY(c, top, n, i)
		_, lines, scale := FitBlock("Effect", c.X(0.5), (c.Y(0.97)-top)/float64(n)-float64(c.SmallText(c.Theme.Mono))*1.6, 1, 0, BlockSmall)
		tag(c, p, r.name, c.X(0.03), y, c.Theme.Muted)
		Block{Font: BlockSmall, Scale: scale, Color: c.Theme.Accent, To: rgbp(c.Theme.Accent2), FX: r.fx}.
			Draw(p, strings.Join(lines, "\n"), c.X(0.03), y+float64(c.SmallText(c.Theme.Mono))*1.2)
	}
}

func slideBlockFXA() Slide {
	return Slide{Title: "Block effects A", Transition: TransitionDissolve,
		View: func(c Ctx, sc *Scene) {
			p, th := sc.Px, c.Theme
			top := heading(c, p, "Block effects A")
			w := BlockSmall.Width("Effect")
			blockFXRows(c, p, top, []struct {
				name string
				fx   func(BlockCell) BlockFX
			}{
				{"BlockDecrypt", BlockDecrypt(c.T, 1.4, th.Faint)},
				{"BlockRain", BlockRain(c.T)},
				{"BlockBeam", BlockBeam(math.Mod(c.T, 3), 1.2)},
				{"BlockSlide", BlockSlide(c.T, w)},
			})
		}}
}

func slideBlockFXB() Slide {
	return Slide{Title: "Block effects B", Transition: TransitionWipe,
		View: func(c Ctx, sc *Scene) {
			p, th := sc.Px, c.Theme
			top := heading(c, p, "Block effects B")
			blockFXRows(c, p, top, []struct {
				name string
				fx   func(BlockCell) BlockFX
			}{
				{"BlockType", BlockType(c.T, 6)},
				{"BlockGlitch", BlockGlitch(c.T, 0.8)},
				{"BlockFade", BlockFade(c.T, 1)},
				{"BlockChain", BlockChain(BlockDecrypt(c.T, 1, th.Muted), BlockBeam(math.Mod(c.T, 2.5), 1), BlockFade(c.T, 0.6), BlockGlitch(c.T-3, 0.3))},
			})
		}}
}

func slideFigFont() Slide {
	return Slide{Title: "FigFont API", Transition: TransitionNone,
		View: func(c Ctx, sc *Scene) {
			p, th := sc.Px, c.Theme
			top := heading(c, p, "FigFont API")
			ss := c.SmallText(th.Mono)
			y := top
			line := func(s string, col RGB) {
				_, h := tag(c, p, s, c.X(0.03), y, col)
				y += h * 1.3
			}
			quote := "Agents aren't users"
			line(fmt.Sprintf("DropQuotes %q", BlockSolid.DropQuotes(quote)), th.Text)
			for _, f := range []*FigFont{BlockShadow, BlockSolid, BlockSmall, BlockHuge, BlockFancy} {
				line(fmt.Sprintf("%-22s rows=%d height=%d width=%d has(digits)=%v has(quote)=%v has(punct)=%v",
					f.Name, f.Rows(), f.Height, f.Width("Hello"), f.Has("2025"), f.Has(quote), f.Has("a, b! c?")), th.Muted)
			}
			wrapped := BlockSmall.Wrap("one two three four\nfive six seven eight nine", 30)
			line(fmt.Sprintf("Wrap: %q", wrapped), th.Accent2)
			// Wrapped lines drawn in the font itself.
			b := Block{Font: BlockSmall, Scale: 1, Color: th.Accent}
			bw, bh := b.Draw(p, strings.Join(wrapped, "\n"), c.X(0.03), y+float64(ss))
			cw, ch := b.Cells(strings.Join(wrapped, "\n"))
			p.Rect(c.X(0.03), y+float64(ss)+bh, bw, 1, th.Faint, 1)
			line("", th.Text)
			line(fmt.Sprintf("cells %dx%d", cw, ch), th.Good)

			// Lowercase and missing characters fall back (to the capital, or to ?).
			lx := c.X(0.6)
			for i, f := range []*FigFont{BlockShadow, BlockSolid, BlockSmall, BlockHuge, BlockFancy} {
				Block{Font: f, Scale: 0.5, Color: th.Muted}.Draw(p, "abc é~", lx, c.Y(0.3)+float64(i)*c.Y(0.05)*1.4)
			}

			// A font loaded from disk through LoadFigFont, and a face from
			// ParseFont, which is what LoadFont wraps.
			fig := LoadFigFont(os.DirFS("fonts/figlet"), "ANSI Shadow.flf")
			Block{Font: fig, Scale: 1, Color: th.Good}.Draw(p, "Disk", c.X(0.6), top)
			data, err := os.ReadFile("fonts/SpaceGrotesk-Bold.ttf")
			if err != nil {
				panic(err)
			}
			pf, err := ParseFont(data)
			if err != nil {
				panic(err)
			}
			lf := LoadFont(os.DirFS("fonts"), "JetBrainsMono-ExtraBold.ttf")
			Text{Font: pf, Size: max(c.Size(0.07), 6), Color: th.Text}.Draw(p, "ParseFont", c.X(0.6), c.Y(0.6))
			Text{Font: lf, Size: max(c.Size(0.07), 6), Color: th.Text}.Draw(p, "LoadFont", c.X(0.6), c.Y(0.75))
		}}
}

func slideComponents() Slide {
	return Slide{Title: "Components", Transition: TransitionPush,
		View: func(c Ctx, sc *Scene) {
			p, th := sc.Px, c.Theme
			top := heading(c, p, "Components")
			pw, ph := c.X(0.22), c.Y(0.2)

			// Panels: with an edge, without, translucent, and a wrapped label.
			Panel(c, p, c.X(0.03), top, pw, ph, "panel", th.Panel, th.Accent2, th.Text, 1)
			Panel(c, p, c.X(0.28), top, pw, ph, "no edge", th.Faint, RGB{}, th.Text, 1)
			Panel(c, p, c.X(0.53), top, pw, ph, "half alpha", th.Accent, th.Warn, th.Text, 0.5)
			Panel(c, p, c.X(0.78), top, pw*0.9, ph, "a label that needs several lines", th.Panel, th.Good, th.Text, Ease(c.T, 1))

			// Arrows in all directions, revealed by time.
			ay := top + ph + c.Y(0.08)
			prog := Ease(c.T, 1.2)
			Arrow(p, c.X(0.05), ay, c.X(0.25), ay, 2, th.Muted, prog)
			Arrow(p, c.X(0.3), ay+c.Y(0.1), c.X(0.45), ay-c.Y(0.03), 3, th.Accent, prog)
			Arrow(p, c.X(0.55), ay-c.Y(0.03), c.X(0.5), ay+c.Y(0.1), 2, th.Good, Progress(c.T, 0.3, 1))
			Arrow(p, c.X(0.6), ay, c.X(0.6), ay, 2, th.Warn, 1) // zero length
			Arrow(p, c.X(0.65), ay, c.X(0.75), ay, 2, th.Warn, 0)
			Arrow(p, c.X(0.8), ay, c.X(0.95), ay, 2, th.Accent2, 2) // clamped
			LineLabel(c, p, th.Mono, "call", c.X(0.15), ay, th.Text, 1)
			LineLabel(c, p, th.Body, "faded", c.X(0.375), ay+c.Y(0.035), th.Accent, 0.5)

			// Labels in every alignment, chips of several kinds.
			ly := ay + c.Y(0.16)
			Label(c, p, "left label", c.X(0.03), ly, th.Text, Left)
			Label(c, p, "centered", c.X(0.22), ly, th.Muted, Center)
			Label(c, p, "right label", c.X(0.5), ly, th.Accent2, Right)
			cx := c.X(0.55)
			for i, s := range []string{"chip", "another chip", "x"} {
				w := Chip(c, p, s, cx, ly-c.Unit(0.01), th.Background, []RGB{th.Good, th.Accent, th.Accent2}[i], 1-0.25*float64(i))
				cx += w + c.Unit(0.02)
			}
			Label(c, p, fmt.Sprintf("chip h=%.1f", ChipHeight(c)), c.X(0.03), ly+ChipHeight(c)+c.Unit(0.01), th.Faint, Left)

			// Speech bubble, placeholder box, illustrative tag.
			by := ly + c.Y(0.1)
			SpeechBubble(p, c.X(0.03), by, c.X(0.2), c.Y(0.1), c.X(0.06), by+c.Y(0.16), th.Panel, th.Accent, 1)
			SpeechBubble(p, c.X(0.27), by, c.X(0.15), c.Y(0.08), c.X(0.4), by+c.Y(0.12), th.Accent2, th.Text, 0.7)
			PlaceholderBox(c, p, c.X(0.45), by, c.X(0.3), c.Y(0.2), "a screenshot goes here")
			IllustrativeTag(c, p, c.X(0.97), by)
			IllustrativeTag(c, p, c.X(0.97), by+c.Y(0.1))
		}}
}

func slideCycle() Slide {
	return Slide{Title: "Cycle", Steps: 5, Transition: TransitionDefault,
		View: func(c Ctx, sc *Scene) {
			p, th := sc.Px, c.Theme
			top := heading(c, p, "Cycle")
			looping, theta, active, lx := CycleDiagram{
				Labels: []string{"Look at the screen", "Think about what to do next", "Act on it", "Learn from the result"},
				Lap:    3, Step0: 1,
			}.Draw(c, p, top+c.Y(0.02))
			// Pin the return values.
			Label(c, p, fmt.Sprintf("loop=%v th=%.2f act=%d", looping, theta, active), c.X(0.03), c.Y(0.93), th.Muted, Left)
			p.Rect(lx, c.Y(0.95), 4, 2, th.Accent, 1)
		}}
}

func slideCycleRing() Slide {
	return Slide{Title: "Cycle ring", Steps: 4, Transition: TransitionWipe,
		View: func(c Ctx, sc *Scene) {
			p, th := sc.Px, c.Theme
			top := heading(c, p, "Cycle ring")
			looping, theta, active, lx := CycleDiagram{
				Labels: []string{"Plan", "Build", "Ship"},
				Lap:    2, Step0: 0, Ring: th.Good,
			}.Draw(c, p, top+c.Y(0.02))
			Label(c, p, fmt.Sprintf("loop=%v th=%.2f act=%d", looping, theta, active), c.X(0.03), c.Y(0.93), th.Muted, Left)
			p.Rect(lx, c.Y(0.95), 4, 2, th.Accent2, 1)
		}}
}

func slideBullets() Slide {
	return Slide{Title: "Bullets", Steps: 5, Transition: TransitionPush,
		View: func(c Ctx, sc *Scene) {
			p, th := sc.Px, c.Theme
			top := heading(c, p, "Bullets")
			BulletList(c, p, []string{"one idea per slide", "put the detail in the notes", "builds reveal a line at a time", "and past lines dim"},
				c.X(0.03), top+c.Y(0.03), c.X(0.45), c.Y(0.6), 1)
			// A second list in a tight box, with long wrapped lines, starting at step 2.
			BulletList(c, p, []string{"a much longer bullet that has to wrap onto a second or third line in the narrow column",
				"short", "another fairly long line that will need wrapping too, to force a smaller text size"},
				c.X(0.55), top+c.Y(0.03), c.X(0.42), c.Y(0.5), 2)
			// A one-line list and an empty one.
			BulletList(c, p, []string{"solo"}, c.X(0.03), c.Y(0.85), c.X(0.3), c.Y(0.1), 0)
			BulletList(c, p, nil, c.X(0.5), c.Y(0.85), c.X(0.3), c.Y(0.1), 0)
			_ = th
		}}
}

func slidePixels() Slide {
	return Slide{Title: "Pixels", Transition: TransitionDissolve,
		View: func(c Ctx, sc *Scene) {
			p, th := sc.Px, c.Theme
			p.Fill(th.Panel)
			p.VGradient(int(c.Y(0.5)), p.H-1, th.Panel, Mix(th.Panel, th.Accent2, 0.35))
			p.VGradient(-5, 3, th.Warn, th.Good) // clipped at the top
			top := heading(c, p, "Pixels")
			u := c.Unit(0.1)

			// Discs, with fractional centers, radii and alpha.
			for i := 0; i < 5; i++ {
				p.Disc(c.X(0.05)+float64(i)*u*1.1+0.5*float64(i)/3, top+u/2+0.37, u*0.2*float64(i+1)/3, th.Accent, 0.4+0.15*float64(i))
			}
			// Arcs: partial and full rings, thick and thin, wrapping past 2pi.
			p.Arc(c.X(0.5), top+u, u*0.8, 3, 0, Lerp(0.5, 2*math.Pi, Ease(c.T, 2)), th.Good, 1)
			p.Arc(c.X(0.5), top+u, u*0.5, 1.2, 1, 4, th.Accent2, 0.8)
			p.Arc(c.X(0.65), top+u, u*0.7, 5, 0, 2*math.Pi, th.Warn, 0.6)
			p.Arc(c.X(0.8), top+u, u*0.7, 2, 4, 2, th.Text, 1) // backwards

			// Lines at assorted angles and widths.
			ly := top + 2.6*u
			for i := 0; i < 8; i++ {
				a := float64(i) * math.Pi / 8
				p.Line(c.X(0.1)-math.Cos(a)*u*0.6, ly+math.Sin(a)*u*0.6, c.X(0.1)+math.Cos(a)*u*0.6, ly-math.Sin(a)*u*0.6, 0.5+float64(i)*0.4, th.Text, 0.9)
			}
			p.Line(c.X(0.25), ly, c.X(0.25), ly, 4, th.Accent, 1) // a dot

			// Rects (fractional edges) and round rects, filled and outlined.
			p.Rect(c.X(0.3)+0.3, ly-u*0.5+0.6, u*1.2, u*0.8, th.Accent, 0.9)
			p.Rect(c.X(0.4), ly-u*0.5, u*0.9, u*0.8, th.Good, 0.35)
			p.RoundRect(c.X(0.5), ly-u*0.5, u*1.4, u*0.9, u*0.3, 0, th.Accent2, 1)
			p.RoundRect(c.X(0.62), ly-u*0.5, u*1.4, u*0.9, u*0.3, 3, th.Accent2, 1)
			p.RoundRect(c.X(0.74), ly-u*0.5, u*1.4, u*0.9, 100, 1.5, th.Warn, 0.8) // radius clamped
			p.RoundRect(c.X(0.86), ly-u*0.5, u*0.7, u*0.7, 0, 2, th.Text, 1)

			// Glow, additive Add, Blend, Set, At.
			gy := ly + 2*u
			p.Glow(c.X(0.1), gy, u*1.2, th.Accent, 0.9)
			p.Glow(c.X(0.2), gy, u*0.8, th.Accent2, 0.6+0.4*Pulse(c.T, 2))
			for x := 0; x < 40; x++ {
				p.Add(int(c.X(0.3))+x, int(gy), th.Good, float64(x)/40)
				p.Add(int(c.X(0.3))+x, int(gy)+1, RGB{200, 200, 200}, float64(x)/40) // saturates
				p.Blend(int(c.X(0.3))+x, int(gy)+3, th.Warn, float64(x)/39)
				p.Set(int(c.X(0.3))+x, int(gy)+5, Mix(th.Accent, th.Accent2, float64(x)/39))
			}
			p.Set(-1, -1, th.Text)        // out of bounds: ignored
			p.Blend(p.W+3, 0, th.Text, 1) // ditto
			p.Add(0, p.H+2, th.Text, 1)   // ditto
			at := p.At(int(c.X(0.3))+20, int(gy)+5)
			p.Rect(c.X(0.45), gy, u*0.5, u*0.5, at, 1) // copies a pixel it read back
			oob := p.At(-3, 4)
			p.Rect(c.X(0.5), gy, u*0.5, u*0.5, oob, 1)

			// Box clips to the canvas.
			x0, y0, x1, y1 := p.Box(c.X(0.6), gy-u, c.X(0.6)+u*2, gy+u)
			p.Rect(float64(x0), float64(y0), float64(x1-x0), 1, th.Text, 1)
			bx0, by0, bx1, by1 := p.Box(-10, -10, float64(p.W)+10, float64(p.H)+10)
			p.Rect(float64(bx0), float64(by1), 3, 1, th.Good, 1)
			_, _ = by0, bx1
			_ = y1
		}}
}

func slidePixelArt() Slide {
	walk := PixelArt{Rows: []string{
		"..kkkk..",
		".kyyyyk.",
		".kyeyek.",
		".kyyyyk.",
		"..kkkk..",
		".rrrrrr.",
		"rr.rr.rr",
		"...rr...",
		"..r..r..",
	}, Colors: map[rune]RGB{'k': Hex("#222222"), 'y': Hex("#FFD060"), 'e': Hex("#101020"), 'r': Hex("#E04040")}}
	step := walk
	step.Rows = append([]string(nil), walk.Rows...)
	step.Rows[8] = ".r....r."
	step.Rows[7] = "..r..r.."
	return Slide{Title: "Pixel art", Transition: TransitionNone,
		View: func(c Ctx, sc *Scene) {
			p, th := sc.Px, c.Theme
			top := heading(c, p, "Pixel art")
			aw, ah := walk.Size()
			// Frames alternate with time, at whole and fractional scales, flipped.
			frame := walk
			if int(c.T*4)%2 == 1 {
				frame = step
			}
			x := c.X(0.03)
			for i, k := range []float64{1, 1.5, 2.5, 4.25} {
				p.Art(frame, x, top, k, 1, false)
				p.Art(frame, x, top+float64(ah)*k+3, k, 0.6+0.1*float64(i), true)
				x += float64(aw)*k + c.Unit(0.03)
			}
			Label(c, p, fmt.Sprintf("art %dx%d", aw, ah), c.X(0.5), top, th.Muted, Left)
			// A walker crossing the screen, with a flip at the edge, and clipped off the left.
			wx := Lerp(-float64(aw)*3, c.X(1), math.Mod(c.T/6, 1))
			p.Art(frame, wx, c.Y(0.5), 3, 1, int(c.T/6)%2 == 1)

			// A custom per-pixel shape using Box and Coverage: a ring and a squircle.
			cx, cy, r := c.X(0.3), c.Y(0.78), c.Unit(0.14)
			x0, y0, x1, y1 := p.Box(cx-r-2, cy-r-2, cx+r+2, cy+r+2)
			for py := y0; py <= y1; py++ {
				for px := x0; px <= x1; px++ {
					dx, dy := float64(px)+0.5-cx, float64(py)+0.5-cy
					d := math.Abs(math.Hypot(dx, dy)-r) - 2
					p.Blend(px, py, th.Accent, Coverage(d)*0.9)
					q := math.Pow(math.Pow(math.Abs(dx), 4)+math.Pow(math.Abs(dy), 4), 0.25) - r*0.5
					p.Blend(px, py, th.Accent2, Coverage(q)*0.7)
				}
			}
			// Coverage ramp.
			for i := 0; i <= 40; i++ {
				d := float64(i-20) / 10
				p.Rect(c.X(0.6)+float64(i)*2, c.Y(0.7), 2, c.Unit(0.05), th.Text, Coverage(d))
			}
			Label(c, p, fmt.Sprintf("cov %.3f %.3f %.3f", Coverage(-1), Coverage(0), Coverage(1)), c.X(0.6), c.Y(0.8), th.Muted, Left)
		}}
}

func slideImages(ims *Images) Slide {
	return Slide{Title: "Images", Transition: TransitionPush,
		View: func(c Ctx, sc *Scene) {
			p, th := sc.Px, c.Theme
			top := heading(c, p, "Images")
			// The same small image up- and down-scaled, at several alphas.
			boxes := []struct {
				name       string
				x, y, w, h float64
				alpha      float64
			}{
				{"grad.png", c.X(0.03), top, c.X(0.3), c.Y(0.35), 1},
				{"grad.png", c.X(0.36), top, c.X(0.08), c.Y(0.1), 1},
				{"grad.png", c.X(0.47), top, c.X(0.2), c.Y(0.3), 0.5},
				{"wide.png", c.X(0.03), top + c.Y(0.4), c.X(0.6), c.Y(0.3), 1},
				{"wide.png", c.X(0.7), top + c.Y(0.4), c.X(0.07), c.Y(0.04), 0.8},
				{"ramp.png", c.X(0.7), top, c.X(0.27), c.Y(0.3), 1},
				{"ramp.png", c.X(0.8), top + c.Y(0.45), c.X(0.15), c.Y(0.2), Ease(c.T, 1)},
				{"missing.png", c.X(0.03), c.Y(0.85), c.X(0.1), c.Y(0.1), 1},
				{"grad.png", c.X(0.2), c.Y(0.85), 0.5, 0.5, 1}, // too small: not drawn
			}
			for _, b := range boxes {
				p.RoundRect(b.x-1, b.y-1, b.w+2, b.h+2, 1, 1, th.Faint, 1)
				dx, dy, dw, dh, ok := ims.Draw(p, b.name, b.x, b.y, b.w, b.h, b.alpha)
				if ok {
					p.Rect(dx, dy-2, dw, 1, th.Accent, 1)
					_ = dh
				} else {
					p.Line(b.x, b.y, b.x+b.w, b.y+b.h, 1, th.Warn, 1)
				}
				_ = dh
			}
			Label(c, p, fmt.Sprintf("has grad=%v missing=%v", ims.Has("grad.png"), ims.Has("missing.png")), c.X(0.3), c.Y(0.9), th.Muted, Left)
		}}
}

// slideImageAlpha draws a transparent logo over stripes (so the see-through
// parts show), then wide.png cover-cropped into a tall, a wide and a tiny box.
func slideImageAlpha(ims *Images) Slide {
	return Slide{Title: "Image alpha", Transition: TransitionDefault,
		View: func(c Ctx, sc *Scene) {
			p, th := sc.Px, c.Theme
			top := heading(c, p, "Transparent images and cover")
			for i := 0; i < 12; i++ {
				col := th.Panel
				if i%2 == 0 {
					col = th.Faint
				}
				p.Rect(c.X(0.03)+float64(i)*c.X(0.02), top, c.X(0.02), c.Y(0.4), col, 1)
			}
			for i, a := range []float64{1, 0.5} {
				ims.Draw(p, "logo.png", c.X(0.03)+float64(i)*c.X(0.12), top+c.Y(0.05), c.X(0.1), c.Y(0.3), a)
			}
			// A shrunk copy: edges must stay bright, not darken toward black.
			ims.Draw(p, "logo.png", c.X(0.28), top+c.Y(0.05), c.X(0.03), c.Y(0.05), 1)

			boxes := []struct {
				x, y, w, h float64
				alpha      float64
			}{
				{c.X(0.4), top, c.X(0.12), c.Y(0.4), 1},
				{c.X(0.55), top, c.X(0.4), c.Y(0.15), 1},
				{c.X(0.55), top + c.Y(0.2), c.X(0.2), c.Y(0.2), 0.6},
				{c.X(0.8), top + c.Y(0.2), 3, 2, 1},
			}
			for _, b := range boxes {
				p.RoundRect(b.x-1, b.y-1, b.w+2, b.h+2, 1, 1, th.Accent, 1)
				ims.DrawCover(p, "wide.png", b.x, b.y, b.w, b.h, b.alpha)
			}
			// Cover a transparent image: the logo fills the box and crops its sides.
			ims.DrawCover(p, "logo.png", c.X(0.03), top+c.Y(0.5), c.X(0.4), c.Y(0.2), 1)
			_, _, _, _, ok := ims.DrawCover(p, "missing.png", 0, 0, 10, 10, 1)
			Label(c, p, fmt.Sprintf("cover missing ok=%v", ok), c.X(0.5), c.Y(0.9), th.Muted, Left)
		}}
}

func slideMotion() Slide {
	return Slide{Title: "Motion", Transition: TransitionWipe,
		View: func(c Ctx, sc *Scene) {
			p, th := sc.Px, c.Theme
			top := heading(c, p, "Motion and color")
			curves := []struct {
				name string
				f    func(x float64) float64
			}{
				{"Ease", func(x float64) float64 { return Ease(x, 1) }},
				{"EaseOutBack", EaseOutBack},
				{"EaseInOutCubic", EaseInOutCubic},
				{"EaseOutCubic", EaseOutCubic},
				{"Spring", func(x float64) float64 { return Spring(0, 1, x*2, 6, 0.4) }},
				{"Pulse", func(x float64) float64 { return Pulse(x, 0.5) }},
				{"Lerp", func(x float64) float64 { return Lerp(0.2, 0.9, x) }},
				{"Progress", func(x float64) float64 { return Progress(x, 0.25, 0.5) }},
			}
			cw, ch := c.X(0.2), c.Y(0.2)
			// A looping clock for the markers.
			clock := math.Mod(c.T/3, 1)
			for i, cv := range curves {
				x0 := c.X(0.03) + float64(i%4)*(cw+c.X(0.04))
				y0 := top + c.Y(0.03) + float64(i/4)*(ch+c.Y(0.1))
				p.RoundRect(x0, y0, cw, ch, 1, 1, th.Faint, 1)
				for k := 0; k <= 60; k++ {
					x := float64(k) / 60
					p.Disc(x0+x*cw, y0+ch-cv.f(x)*ch*0.8-ch*0.1, 1, th.Accent2, 1)
				}
				p.Disc(x0+clock*cw, y0+ch-cv.f(clock)*ch*0.8-ch*0.1, 2.5, th.Accent, 1)
				Label(c, p, cv.name, x0, y0+ch+2, th.Muted, Left)
			}
			// LerpInt cell steps, Clamp01, Hash01 noise, Mix/Scale/Hex strips.
			y := top + c.Y(0.03) + 2*(ch+c.Y(0.1))
			for i := 0; i < 8; i++ {
				w := float64(LerpInt(2, 40, Clamp01(float64(i)/7-0.1+clock*0.2)))
				p.Rect(c.X(0.03), y+float64(i)*3, w, 2, th.Good, 1)
			}
			for gy := 0; gy < 6; gy++ {
				for gx := 0; gx < 16; gx++ {
					p.Rect(c.X(0.2)+float64(gx)*4, y+float64(gy)*4, 3, 3, th.Text, Hash01(gx, gy, 5))
				}
			}
			a, b := Hex("#FF7A00"), Hex("#40C0FF")
			for i := 0; i < 20; i++ {
				f := float64(i) / 19
				p.Rect(c.X(0.45)+float64(i)*5, y, 5, 6, Mix(a, b, f), 1)
				p.Rect(c.X(0.45)+float64(i)*5, y+7, 5, 6, a.Scale(f*1.5), 1)
				p.Rect(c.X(0.45)+float64(i)*5, y+14, 5, 6, b.Scale(0.3+f), 1)
			}
			// Edge cases: a zero-length Progress, and a spring too slow to settle.
			p.Rect(c.X(0.9), y, 3+4*Progress(c.T, 1, 0), 3, th.Good, 1)
			p.Rect(c.X(0.9), y+5, 3+20*Spring(0, 1, c.T*8, 0.4, 0.995), 3, th.Accent2, 1)
			col := th.Accent.Color()
			r, g, bb, _ := col.RGBA()
			Label(c, p, fmt.Sprintf("hex %d %d %d", r>>8, g>>8, bb>>8), c.X(0.45), y+22, th.Muted, Left)
		}}
}

func slideScene() Slide {
	return Slide{Title: "Character layer", HideChrome: true, Transition: TransitionNone,
		View: func(c Ctx, sc *Scene) {
			th := c.Theme
			sc.Px.VGradient(0, sc.Px.H-1, th.Background, th.Panel)

			box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(th.Accent.Color()).
				Foreground(th.Text.Color()).Padding(0, 1).Render("terminal text\nsecond line")
			// Slides in from beyond the left edge: clipped while it moves.
			sc.Put(LerpInt(-24, 3, Ease(c.T, 1)), 2, box)
			sc.PutCenter(0, 3, box)
			sc.PutCenter(c.W/2+4, -1, box) // runs off the right and top

			sc.Text(1, 1, "plain", th.Muted.Color())
			sc.Text(10, 1, "bold italic", th.Accent.Color(), uv.AttrBold, uv.AttrItalic)
			sc.Text(24, 1, "faint", th.Text.Color(), uv.AttrFaint)
			sc.Text(c.W-6, 1, "clipped at the edge", th.Warn.Color())
			sc.Text(2, 5, "日本語のテキスト 🎉 ✓ wide", th.Good.Color())
			sc.Text(-3, 6, "off the left", th.Muted.Color())

			sc.Fill(2, 8, 12, 3, th.Faint.Color())
			sc.Cell(3, 9, "★", uv.Style{Fg: th.Accent.Color(), Bg: th.Panel.Color(), Attrs: uv.AttrBold})
			sc.Cell(5, 9, "界", uv.Style{Fg: th.Accent2.Color()})
			sc.Cell(-1, -1, "x", uv.Style{}) // ignored
			sc.Overlay(8, 8, lipgloss.NewStyle().Background(th.Warn.Color()).Render(" ov ")+"\n  gap  \n"+lipgloss.NewStyle().Foreground(th.Good.Color()).Render("lay"))
			sc.Put(8, 12, "  put  \n  put  ") // opaque spaces

			bee := Sprite{
				Art:     []string{`(o)##>`, ` ^^^^ `},
				Paint:   []string{`kkkyyk`, `  yy  `},
				Colors:  map[rune]color.Color{'k': Hex("#222222").Color(), 'y': th.Accent.Color()},
				Default: th.Text.Color(),
				Bold:    true,
			}
			sw, sh := bee.Size()
			bx := (int(c.T*8) % max(c.W, 1))
			sc.Sprite(bx, c.H-6, bee)
			sc.Sprite(bx+sw+2, c.H-6, Sprite{Art: []string{"plain", "sprite"}})
			sc.Text(1, c.H-3, fmt.Sprintf("sprite %dx%d", sw, sh), th.Muted.Color())

			// Off-screen scenes: draw into one, copy its pixels out, and
			// release it.
			off := NewScene(24, 5, th)
			off.Px.Disc(8, 5, 6, th.Accent, 1)
			off.Px.Glow(30, 5, 10, th.Accent2, 0.8)
			for y := 0; y < off.Px.H; y++ {
				for x := 0; x < off.Px.W; x++ {
					sc.Px.Blend(c.W/2-4+x, c.H*2-18+y, off.Px.At(x, y), 1)
				}
			}
			off.Release()

			off2 := NewScene(8, 3, th)
			off2.Px.RoundRect(2, 2, 12, 8, 2, 0, th.Good, 1)
			for y := 0; y < off2.Px.H; y++ {
				for x := 0; x < off2.Px.W; x++ {
					sc.Px.Blend(c.W-14+x, c.H*2-14+y, off2.Px.At(x, y), 0.8)
				}
			}
			off2.Release()
		}}
}

func slideBuilds() Slide {
	return Slide{Title: "Builds", Steps: 4, Notes: "four builds: panel, arrow, chip", Hold: 2, Transition: TransitionPush,
		View: func(c Ctx, sc *Scene) {
			p, th := sc.Px, c.Theme
			top := heading(c, p, "Builds")
			Label(c, p, fmt.Sprintf("step %d  since0 %.2f  since1 %.2f  since3 %.2f  stepT %.2f  tdur %.2f",
				c.Step, math.Min(c.Since(0), 99), math.Min(c.Since(1), 99), math.Max(math.Min(c.Since(3), 99), -9), c.StepT, TransitionDuration),
				c.X(0.03), top, th.Muted, Left)
			y := top + c.Y(0.1)
			if c.Reached(1) {
				a := Ease(c.Since(1), 0.4)
				Panel(c, p, c.X(0.05), y, c.X(0.25)*a+1, c.Y(0.25), "first", th.Panel, th.Accent2, th.Text, a)
			}
			if c.Reached(2) {
				Arrow(p, c.X(0.32), y+c.Y(0.12), c.X(0.55), y+c.Y(0.12), 3, th.Accent, Progress(c.Since(2), 0, 0.6))
			}
			if c.Reached(3) {
				s := EaseOutBack(Progress(c.Since(3), 0, 0.5))
				Chip(c, p, "done", c.X(0.6), y+c.Y(0.12)-ChipHeight(c)/2, th.Background, th.Good, Clamp01(s))
				p.Glow(c.X(0.6), y+c.Y(0.12), c.Unit(0.2)*s, th.Good, 0.5*Pulse(c.StepT, 1.5))
			}
			p.Arc(c.X(0.9), c.Y(0.85), c.Unit(0.08), 3, 0, 2*math.Pi*Progress(c.StepT, 0, 2), th.Accent, 1)
		}}
}

func slideLayout() Slide {
	return Slide{Title: "Layout", Steps: 2, Transition: TransitionDefault,
		View: func(c Ctx, sc *Scene) {
			p, th := sc.Px, c.Theme
			outline := func(r Rect, col RGB) { p.RoundRect(r.X, r.Y, r.W, r.H, 1, 1, col, 1) }

			// A slide template: margins, a header strip, a sidebar, and a body.
			page := c.Frame().Inset(c.X(0.03), c.Y(0.03))
			head, body := page.CutTop(c.Y(0.14))
			foot, body := body.CutBottom(c.Y(0.08))
			side, body := body.CutLeft(body.W * 0.25)
			_, body = body.CutLeft(c.Unit(0.03)) // a gutter
			Text{Font: th.Display, Size: c.Size(0.08), Color: th.Accent}.Draw(p, "Layout", head.X, head.Y)
			outline(head, th.Faint)
			outline(foot, th.Faint)
			Label(c, p, "foot", foot.X+c.Unit(0.01), foot.Y, th.Muted, Left)

			// Weighted rows in the sidebar, a weighted column split and a grid
			// in the body; the grid's cells fill in at step 1.
			for i, r := range side.Rows(c.Unit(0.02), 1, 2, 1) {
				Panel(c, p, r.X, r.Y, r.W, r.H, fmt.Sprintf("row %d", i), th.Panel, th.Accent2, th.Text, 1)
			}
			cols := body.Cols(c.Unit(0.02), 2, 1)
			for i, r := range cols[0].Grid(3, 2, c.Unit(0.02)) {
				outline(r, th.Muted)
				if c.Reached(1) {
					in := LerpRect(r.Anchor(0, 0, 0.5, 0.5), r.Inset(c.Unit(0.01), c.Unit(0.01)), Ease(c.Since(1)-0.05*float64(i), 0.4))
					p.RoundRect(in.X, in.Y, in.W, in.H, c.Unit(0.02), 0, Mix(th.Accent, th.Good, float64(i)/5), 1)
				}
			}

			// Place anchors a box at each corner and the middle of the column.
			right := cols[1]
			outline(right, th.Faint)
			cw, ch := right.W*0.3, right.H*0.15
			for _, a := range [][2]float64{{0, 0}, {1, 0}, {0.5, 0.5}, {0, 1}, {1, 1}} {
				r := right.Anchor(cw, ch, a[0], a[1])
				p.Rect(r.X, r.Y, r.W, r.H, th.Accent2, 0.6)
			}
			x, y := right.Center()
			p.Disc(x, y, c.Unit(0.01), th.Warn, 1)
			oversize := right.Anchor(right.W*1.2, ch, 0.5, 0.25) // overhangs both sides
			p.Rect(oversize.X, oversize.Y, oversize.W, oversize.H, th.Good, 0.3)

			// Ctx.Rect, Sub, Right and Bottom, with a negative Inset growing a box.
			tag := c.Rect(0.8, 0.04, 0.17, 0.08)
			outline(tag.Inset(-2, -2), th.Accent)
			dot := tag.Sub(0.9, 0.1, 0.05, 0.3)
			p.Disc(dot.Right(), dot.Bottom(), 2, th.Accent, 1)
		}}
}

// slideMorph is a pair of slides for TransitionMorph: the second enters with
// it, so the elements both place (title, box, dot) glide to their new rects
// while "before" fades out and "after" fades in.
func slideMorph(after bool) Slide {
	title, tr := "Morph A", TransitionDefault
	if after {
		title, tr = "Morph B", TransitionMorph.Over(1)
	}
	return Slide{Title: title, Transition: tr,
		View: func(c Ctx, sc *Scene) {
			th := c.Theme
			sc.Px.VGradient(0, sc.Px.H-1, th.Background, th.Panel)
			page := c.Frame().Inset(c.X(0.04), c.Y(0.05))

			head, body := page.CutTop(page.H * 0.5)
			box, dot := body.Inset(0, c.Unit(0.03)).CutLeft(body.W * 0.6)
			if after {
				head, body = page.CutTop(page.H * 0.18)
				dot, box = body.CutLeft(body.W * 0.25)
				box = box.Inset(c.Unit(0.03), c.Unit(0.03))
			}
			sc.Place("title", head, func(p *Pixels, r Rect) {
				size, s := th.Display.Fit(title, r.W, r.H, c.Size(0.3), 0)
				Text{Font: th.Display, Size: size, Color: th.Accent, Align: Center, Glow: 0.4}.Draw(p, s, r.X+r.W/2, r.Y)
			})
			sc.Place("box", box, func(p *Pixels, r Rect) {
				fill := th.Panel
				if after {
					fill = th.Accent2.Scale(0.5)
				}
				Panel(c, p, r.X, r.Y, r.W, r.H, "the same panel", fill, th.Accent2, th.Text, 1)
			})
			sc.Place("dot", dot.Anchor(dot.H*0.5, dot.H*0.5, 0.5, 0.5), func(p *Pixels, r Rect) {
				x, y := r.Center()
				p.Disc(x, y, r.W/2, th.Good, 1)
			})
			only, label := "before", NewRect(page.X, page.Bottom()-c.Y(0.1), page.W*0.3, c.Y(0.1))
			if after {
				only, label = "after", NewRect(page.Right()-page.W*0.3, page.Bottom()-c.Y(0.1), page.W*0.3, c.Y(0.1))
			}
			sc.Place(only, label, func(p *Pixels, r Rect) { Label(c, p, only+" only", r.X, r.Y, th.Muted, Left) })
			sc.Place("", c.Rect(0.9, 0.9, 0.05, 0.05), func(p *Pixels, r Rect) { p.Rect(r.X, r.Y, r.W, r.H, th.Warn, 1) })
		}}
}

// slidePosition draws what an overlay would from Ctx's position: the page
// number in each alignment, the progress bar and the section. The theme's own
// Overlay stays as it was, so the other slides' frames don't change.
func slidePosition() Slide {
	return Slide{Title: "Position", Section: "Overlays",
		View: func(c Ctx, sc *Scene) {
			p, th := sc.Px, c.Theme
			top := heading(c, p, "Position")
			tag(c, p, fmt.Sprintf("Index %d  Count %d  Section %q", c.Index, c.Count, c.Section), c.X(0.03), top, th.Muted)

			y := top + c.Y(0.15)
			PageNumber(c, p, c.X(0.03), y, Left, th.Text)
			PageNumber(c, p, c.X(0.5), y, Center, th.Accent2)
			PageNumber(c, p, c.X(0.97), y, Right, th.Good)
			ProgressBar(c, p, c.Rect(0.03, 0.55, 0.94, 0.03), th.Accent, th.Faint)
			ProgressBar(c, p, c.Rect(0.03, 0.65, 0.4, 0.015), th.Good, th.Panel)

			// A bar and a page number in the corner, as an overlay would.
			ProgressBar(c, p, NewRect(0, c.PH()-c.Unit(0.01), c.PW(), c.Unit(0.01)), th.Accent2, th.Faint)
			Label(c, p, c.Section, c.X(0.03), c.Y(0.9), th.Muted, Left)
			PageNumber(c, p, c.X(0.97), c.Y(0.9), Right, th.Muted)

			// An unpositioned Ctx draws no number and an empty track.
			bare := c
			bare.Count = 0
			PageNumber(bare, p, c.X(0.03), c.Y(0.75), Left, th.Warn)
			ProgressBar(bare, p, c.Rect(0.5, 0.75, 0.3, 0.02), th.Warn, th.Faint)
		}}
}
