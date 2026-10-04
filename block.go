package decker

import (
	"math"
	"strings"
	"unicode/utf8"
)

// Block draws text in a block font onto the pixel canvas, scaled to the screen:
// each font cell becomes a Scale×(2·Scale) pixel box, so block letters are as
// big on stage at 682 columns as at 240. FitBlock picks a font and scale.
type Block struct {
	Font  *FigFont
	Scale float64 // pixel width of one font cell, at least 0.5; it is twice as tall

	Color  RGB  // solid parts (█ ▀ ▄ ▌ ▐ ░ …)
	To     *RGB // if set, solid parts get a left-to-right gradient Color → To
	Shadow RGB  // everything else (box-drawing edges); zero = Color dimmed toward the canvas background
	Drop   RGB  // if set, a drop shadow of the solid parts, offset down-right
	Align  Align
	Gap    int // blank font rows between lines

	// Glow draws a soft light behind the solid parts.
	Glow float64

	FX BlockEffect
}

// Cells returns the block's size in font cells for s: what BlockCell.W and
// BlockCell.H will be, and the width effects like BlockSlide travel.
func (b Block) Cells(s string) (w, h int) { return b.dims(strings.Split(s, "\n")) }

func (b Block) dims(lines []string) (w, h int) {
	return b.Font.widest(lines), len(lines)*b.Font.Rows() + (len(lines)-1)*b.Gap
}

// Size returns the block's size in pixels for s.
func (b Block) Size(s string) (w, h float64) {
	cw, ch := b.Cells(s)
	return float64(cw) * b.Scale, float64(ch) * 2 * b.Scale
}

type placedCell struct {
	x, y  float64 // top-left pixel
	r     rune
	c     RGB
	a     float64
	solid bool
}

// Draw renders s (which may contain "\n") with its top edge at pixel y and x as
// the left edge, center or right edge per Align. It returns the size in pixels.
func (b Block) Draw(p *Pixels, s string, x, y float64) (w, h float64) {
	k := math.Max(b.Scale, 0.5)
	lines := strings.Split(s, "\n")
	cw, ch := b.dims(lines)
	w, h = float64(cw)*k, float64(ch)*2*k
	left := x + b.Align.shift(w)
	shadow := b.Shadow
	if shadow == (RGB{}) {
		shadow = Mix(p.BG, b.Color, 0.45)
	}
	placed := b.place(lines, cw, ch, left, y, k, shadow)

	b.glow(p, placed, left, y, k, w, h)
	if b.Drop != (RGB{}) {
		off := math.Max(1, math.Round(k*0.7))
		for _, c := range placed {
			if c.solid {
				drawBlockRunePx(p, c.r, c.x+off, c.y+off, k, b.Drop, c.a)
			}
		}
	}
	for _, c := range placed {
		drawBlockRunePx(p, c.r, c.x, c.y, k, c.c, c.a)
	}
	return w, h
}

// place lays out lines, runs the effects, and returns the cells that stay
// visible.
func (b Block) place(lines []string, cw, ch int, left, y, k float64, shadow RGB) []placedCell {
	var cells []placedCell
	row0, chars := 0, 0
	for li, l := range lines {
		if li > 0 {
			row0 += b.Gap
		}
		rows := b.Font.render(l)
		pad := 0
		switch b.Align {
		case Center:
			pad = (cw - rows.width()) / 2
		case Right:
			pad = cw - rows.width()
		}
		for ry, row := range rows {
			for rx, fc := range row {
				if fc.r == ' ' {
					continue
				}
				col := pad + rx
				cell := BlockCell{
					Char: chars + fc.owner, Col: col, Row: row0 + ry,
					W: cw, H: ch, U: float64(col) / math.Max(float64(cw-1), 1),
					Rune: fc.r, Solid: isSolid(fc.r), Line: li, Line0: chars,
				}
				fx := BlockFX{Alpha: 1}
				if b.FX != nil {
					fx = b.FX(cell)
				}
				if fx.Alpha <= 0.02 {
					continue
				}
				c := shadow
				if cell.Solid {
					c = b.Color
					if b.To != nil {
						c = Mix(b.Color, *b.To, cell.U)
					}
				}
				if fx.Color != nil {
					c = *fx.Color
				}
				if fx.Bright > 0 {
					c = Mix(c, RGB{255, 255, 255}, fx.Bright)
				}
				r := fc.r
				if fx.Rune != 0 {
					r = fx.Rune
				}
				cells = append(cells, placedCell{
					x: left + (float64(col)+fx.DX)*k, y: y + (float64(row0+ry)+fx.DY)*2*k,
					r: r, c: c, a: math.Min(fx.Alpha, 1), solid: isSolid(r),
				})
			}
		}
		row0 += len(rows)
		chars += utf8.RuneCountInString(l) + 1
	}
	return cells
}

func (b Block) glow(p *Pixels, cells []placedCell, left, y, k, w, h float64) {
	if b.Glow <= 0 {
		return
	}
	pad := int(math.Ceil(k * 4))
	cov := newCoverage(int(left)-pad, int(y)-pad, int(w)+2*pad, int(h)+2*pad)
	for _, c := range cells {
		if !c.solid {
			continue
		}
		x0, y0 := int(c.x-left)+pad, int(c.y-y)+pad
		for yy := max(y0, 0); yy < min(y0+int(2*k), cov.h); yy++ {
			for xx := max(x0, 0); xx < min(x0+int(math.Ceil(k)), cov.w); xx++ {
				cov.a[yy*cov.w+xx] = float32(c.a)
			}
		}
	}
	gc := b.Color
	if b.To != nil {
		gc = Mix(b.Color, *b.To, 0.5)
	}
	cov.addGlow(p, max(int(k*1.5), 2), gc, b.Glow*0.6)
}
