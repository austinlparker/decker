package decker

import (
	"math"
	"strings"
)

// Align positions text relative to the x coordinate passed to Draw.
type Align int

const (
	Left   Align = iota // x is the left edge
	Center              // x is the center
	Right               // x is the right edge
)

func (a Align) shift(w float64) float64 {
	switch a {
	case Center:
		return -w / 2
	case Right:
		return -w
	}
	return 0
}

// DefaultLeading is the line height, as a multiple of size, used when Leading is 0.
const DefaultLeading = 1.1

func leadingOr(l float64) float64 {
	if l == 0 {
		return DefaultLeading
	}
	return l
}

// Text draws a block of raster type with a Font; sizes below 4 pixels are drawn at 4.
type Text struct {
	Font    *Font   // required
	Size    int     // pixels
	Color   RGB     // fill color
	To      *RGB    // if set, a left-to-right gradient from Color to To across the block's width
	Align   Align   // how x is interpreted
	Leading float64 // line height as a multiple of Size (default 1.1)

	Glow      float64 // soft glow strength, 0 (none) to ~1.5
	GlowColor *RGB    // defaults to Color

	// MaxW, if set, wraps lines to this width.
	MaxW float64

	// Shine returns extra brightness (0..1) at horizontal position u, 0 at the block's
	// left edge to 1 at its right; see ShineBand.
	Shine func(u float64) float64

	FX GlyphEffect
}

func (t Text) font() *Font {
	if t.Font == nil {
		panic("decker.Text: Font is required")
	}
	return t.Font
}

func (t Text) resolved() (*Font, int) { return t.font().resolve(t.Size) }

// DrawMid draws one line of s with its ink centered vertically on cy (x as for
// Draw). Centering the line box instead leaves lowercase text low and
// descenders poking into borders.
func (t Text) DrawMid(p *Pixels, s string, x, cy float64) (w, h float64) {
	f, size := t.resolved()
	top, bot := f.Ink(s, size)
	if top == bot {
		top, bot = -f.CapHeight(size), 0
	}
	baseline := math.Round(cy - (top+bot)/2)
	return t.Draw(p, s, x, baseline-t.baseOff(f, size))
}

// Baseline is the distance from the top of a line to its baseline, as Draw lays it out.
func (t Text) Baseline() float64 {
	f, size := t.resolved()
	return t.baseOff(f, size)
}

// baseOff is the top-of-line to baseline distance; the 0.08*size nudge centers ink in the line box.
func (t Text) baseOff(f *Font, size int) float64 {
	return f.Ascent(size) + (float64(size)*leadingOr(t.Leading)-float64(size))/2 - float64(size)*0.08
}

// Draw renders s (which may contain "\n") with its top edge at y and returns the
// block's size. With FX, glyph indexes count every rune of every line, spaces
// included, plus one per line break.
func (t Text) Draw(p *Pixels, s string, x, y float64) (w, h float64) {
	f, size := t.resolved()
	if t.MaxW > 0 {
		s = strings.Join(f.Wrap(s, size, t.MaxW), "\n")
	}
	lines := strings.Split(s, "\n")
	lineH := float64(size) * leadingOr(t.Leading)
	widths := make([]float64, len(lines))
	for i, l := range lines {
		widths[i] = f.Measure(l, size)
		w = max(w, widths[i])
	}
	h = lineH * float64(len(lines))
	blockX := x + t.Align.shift(w)

	// The coverage buffer is padded by size on every side for glow and effect motion.
	pad := float64(size)
	cov := newCoverage(int(math.Floor(blockX-pad)), int(math.Floor(y-pad)), int(math.Ceil(w+2*pad))+1, int(math.Ceil(h+2*pad))+1)
	t.stamp(cov, f, size, lines, widths, x, y, lineH)

	if t.Glow > 0 {
		gc := t.Color
		if t.GlowColor != nil {
			gc = *t.GlowColor
		}
		cov.addGlow(p, max(size/6, 2), gc, t.Glow*0.9)
	}
	t.paint(p, cov, blockX, w)
	return w, h
}

func (t Text) stamp(cov coverage, f *Font, size int, lines []string, widths []float64, x, y, lineH float64) {
	baseOff := t.baseOff(f, size)
	gi := 0
	for li, line := range lines {
		lx := x + t.Align.shift(widths[li])
		pen, prev := 0.0, rune(-1)
		base := y + float64(li)*lineH + baseOff
		for _, r := range line {
			if prev >= 0 {
				pen += f.kern(prev, r, size)
			}
			fx := GlyphFX{Alpha: 1}
			if t.FX != nil {
				fx = t.FX(gi)
			}
			g := f.glyph(r, size)
			gx := lx + pen + fx.DX - float64(cov.x0)
			show := r
			if fx.Rune != 0 && r != ' ' {
				// Center the stand-in glyph in the real glyph's advance.
				show = fx.Rune
				gx += (g.adv - f.glyph(show, size).adv) / 2
			}
			ix := math.Floor(gx)
			q := int((gx - ix) * subpixel)
			cov.stamp(f.glyphAt(show, size, q), ix, base+fx.DY-float64(cov.y0), fx.Alpha)
			pen += g.adv
			prev = r
			gi++
		}
		gi++
	}
}

func (t Text) colorAt(u float64) RGB {
	c := t.Color
	if t.To != nil {
		c = Mix(t.Color, *t.To, u)
	}
	if t.Shine != nil {
		c = Mix(c, RGB{255, 255, 255}, t.Shine(u))
	}
	return c
}

func (t Text) paint(p *Pixels, cov coverage, blockX, w float64) {
	for y := 0; y < cov.h; y++ {
		for x := 0; x < cov.w; x++ {
			a := cov.a[y*cov.w+x]
			if a <= 0.002 {
				continue
			}
			ix := cov.x0 + x
			u := 0.0
			if w > 0 {
				u = Clamp01((float64(ix) - blockX) / w)
			}
			p.Blend(ix, cov.y0+y, t.colorAt(u), float64(a))
		}
	}
}
