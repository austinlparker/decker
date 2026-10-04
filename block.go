package decker

import (
	"fmt"
	"math"
	"strings"
	"sync"
)

// Block draws text in a block font (see figlet.go) onto the pixel canvas,
// scaled to the screen: each character cell of the font becomes a
// Scale×(2·Scale) pixel box, so block letters are as big on stage at 682
// columns as at 240. Use FitBlock to pick a font and scale for a box.
type Block struct {
	Font  *FigFont
	Scale float64 // pixel width of one font cell; it is twice as tall

	Color  RGB  // solid parts (█ ▀ ▄ ▌ ▐ ░ …)
	To     *RGB // if set, solid parts get a left-to-right gradient Color → To
	Shadow RGB  // everything else (box-drawing edges); zero = Color dimmed toward the canvas background
	Drop   RGB  // if set, a drop shadow of the solid parts, offset down-right
	Align  Align
	Gap    int // blank font rows between lines

	// Glow draws a soft light behind the solid parts.
	Glow float64

	// FX, if set, animates each cell (see blockfx.go).
	FX func(BlockCell) BlockFX
}

// BlockCell describes one non-blank cell of rendered block text, for effects.
type BlockCell struct {
	Char        int     // index of the source character (across all lines)
	Col, Row    int     // position within the whole block, in font cells
	W, H        int     // block size in font cells
	U           float64 // Col / W: 0 at the left edge, 1 at the right
	Rune        rune
	Solid       bool
	Line, Line0 int // which line, and the char index where it starts
}

// BlockFX is an effect's instruction for one cell.
type BlockFX struct {
	DX, DY float64 // offset in font cells (fractions allowed)
	Alpha  float64 // 0 hides the cell, 1 is fully visible
	Rune   rune    // 0 keeps the real character
	Bright float64 // 0..1 pushes the color toward white
	Color  *RGB    // replaces the color entirely
}

func isSolid(r rune) bool {
	return strings.ContainsRune("█▓▒░▀▄▌▐▖▗▘▝▙▛▜▟▚▞■", r)
}

// Cells returns the block's size in font cells for s: what BlockCell.W and
// BlockCell.H will be, and the width effects like BlockSlide travel.
func (b Block) Cells(s string) (w, h int) {
	for i, l := range strings.Split(s, "\n") {
		w = max(w, b.Font.Width(l))
		if i > 0 {
			h += b.Gap
		}
		h += b.Font.Rows()
	}
	return
}

// Size returns the block's size in pixels for s.
func (b Block) Size(s string) (w, h float64) {
	cw, ch := b.Cells(s)
	return float64(cw) * b.Scale, float64(ch) * 2 * b.Scale
}

// Draw renders s (which may contain "\n") with its top edge at pixel y. x
// is the left edge, center, or right edge according to Align. It returns
// the block's size in pixels.
func (b Block) Draw(p *Pixels, s string, x, y float64) (w, h float64) {
	k := math.Max(b.Scale, 0.5)
	type placed struct {
		x, y  float64 // top-left pixel
		r     rune
		c     RGB
		a     float64
		solid bool
	}
	lines := strings.Split(s, "\n")
	cw, ch := b.Cells(s)
	w, h = float64(cw)*k, float64(ch)*2*k
	left := x
	switch b.Align {
	case Center:
		left = x - w/2
	case Right:
		left = x - w
	}
	shadow := b.Shadow
	if shadow == (RGB{}) {
		shadow = Mix(p.BG, b.Color, 0.45)
	}

	var cells []placed
	row0, chars := 0, 0
	for li, l := range lines {
		if li > 0 {
			row0 += b.Gap
		}
		rows, owner := b.Font.render(l)
		lw := 0
		for _, r := range rows {
			lw = max(lw, len(r))
		}
		pad := 0
		switch b.Align {
		case Center:
			pad = (cw - lw) / 2
		case Right:
			pad = cw - lw
		}
		for ry, row := range rows {
			for rx, r := range row {
				if r == ' ' {
					continue
				}
				col := pad + rx
				cell := BlockCell{
					Char: chars + owner[ry][rx], Col: col, Row: row0 + ry,
					W: cw, H: ch, U: float64(col) / math.Max(float64(cw-1), 1),
					Rune: r, Solid: isSolid(r), Line: li, Line0: chars,
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
				if fx.Rune != 0 {
					r = fx.Rune
				}
				cells = append(cells, placed{
					x: left + (float64(col)+fx.DX)*k, y: y + (float64(row0+ry)+fx.DY)*2*k,
					r: r, c: c, a: math.Min(fx.Alpha, 1), solid: isSolid(r),
				})
			}
		}
		row0 += len(rows)
		chars += len([]rune(l)) + 1
	}

	if b.Glow > 0 {
		pad := int(math.Ceil(k * 4))
		bw, bh := int(w)+2*pad, int(h)+2*pad
		cov := make([]float32, bw*bh)
		for _, c := range cells {
			if !c.solid {
				continue
			}
			x0, y0 := int(c.x-left)+pad, int(c.y-y)+pad
			for yy := max(y0, 0); yy < min(y0+int(2*k), bh); yy++ {
				for xx := max(x0, 0); xx < min(x0+int(math.Ceil(k)), bw); xx++ {
					cov[yy*bw+xx] = float32(c.a)
				}
			}
		}
		gc := b.Color
		if b.To != nil {
			gc = Mix(b.Color, *b.To, 0.5)
		}
		addGlow(p, cov, bw, bh, int(left)-pad, int(y)-pad, max(int(k*1.5), 2), gc, b.Glow*0.6)
	}
	if b.Drop != (RGB{}) {
		off := math.Max(1, math.Round(k*0.7))
		for _, c := range cells {
			if c.solid {
				drawBlockRunePx(p, c.r, c.x+off, c.y+off, k, b.Drop, c.a)
			}
		}
	}
	for _, c := range cells {
		drawBlockRunePx(p, c.r, c.x, c.y, k, c.c, c.a)
	}
	return w, h
}

// drawBlockRunePx paints one block or box-drawing character into the k×2k
// pixel box at (x, y).
func drawBlockRunePx(p *Pixels, r rune, x, y, k float64, c RGB, a float64) {
	cw, ch := k, 2*k
	if m, ok := quadrants[r]; ok {
		// Snap to whole pixels so neighboring cells tile exactly: with
		// antialiased fractional edges, two half-covered pixels at a shared
		// edge don't add up to a solid one, and a faint grid shows through.
		x0, x1 := math.Round(x), math.Round(x+cw)
		y0, y1 := math.Round(y), math.Round(y+ch)
		xm, ym := math.Round(x+cw/2), math.Round(y+ch/2)
		if m == 0b1111 {
			p.Rect(x0, y0, x1-x0, y1-y0, c, a)
			return
		}
		quads := [4][4]float64{{x0, y0, xm, ym}, {xm, y0, x1, ym}, {x0, ym, xm, y1}, {xm, ym, x1, y1}}
		for i, q := range quads {
			if m&(0b1000>>i) != 0 {
				p.Rect(q[0], q[1], q[2]-q[0], q[3]-q[1], c, a)
			}
		}
		return
	}
	x, y = math.Round(x), math.Round(y)
	switch r {
	case '░':
		p.Rect(x, y, cw, ch, c, a*0.35)
		return
	case '▒':
		p.Rect(x, y, cw, ch, c, a*0.55)
		return
	case '▓':
		p.Rect(x, y, cw, ch, c, a*0.8)
		return
	case '■', '◆':
		p.Rect(x+cw*0.2, y+ch*0.3, cw*0.6, ch*0.4, c, a)
		return
	case '·':
		p.Rect(x+cw*0.35, y+ch*0.42, cw*0.3, ch*0.16, c, a)
		return
	}
	arms, ok := blockBoxArms[r]
	if !ok {
		return
	}
	t := math.Max(1, k*0.3)
	cx, cy := x+cw/2, y+ch/2
	stroke := func(ox, oy float64) {
		if arms.l {
			p.Rect(x, cy+oy-t/2, cw/2+ox+t/2, t, c, a)
		}
		if arms.r {
			p.Rect(cx+ox-t/2, cy+oy-t/2, cw/2-ox+t/2, t, c, a)
		}
		if arms.u {
			p.Rect(cx+ox-t/2, y, t, ch/2+oy+t/2, c, a)
		}
		if arms.d {
			p.Rect(cx+ox-t/2, cy+oy-t/2, t, ch/2-oy+t/2, c, a)
		}
	}
	if arms.double {
		o := math.Max(k*0.22, t*0.8)
		stroke(-o, -o)
		stroke(o, o)
		return
	}
	stroke(0, 0)
}

// quadrants maps block characters to the quarters they fill:
// bit 3 top-left, 2 top-right, 1 bottom-left, 0 bottom-right.
var quadrants = map[rune]uint8{
	'█': 0b1111, '▀': 0b1100, '▄': 0b0011, '▌': 0b1010, '▐': 0b0101,
	'▘': 0b1000, '▝': 0b0100, '▖': 0b0010, '▗': 0b0001,
	'▛': 0b1110, '▜': 0b1101, '▙': 0b1011, '▟': 0b0111,
	'▚': 0b1001, '▞': 0b0110,
}

type arms struct{ u, d, l, r, double bool }

var blockBoxArms = map[rune]arms{
	'─': {l: true, r: true}, '│': {u: true, d: true},
	'┌': {r: true, d: true}, '┐': {l: true, d: true}, '└': {u: true, r: true}, '┘': {u: true, l: true},
	'├': {u: true, d: true, r: true}, '┤': {u: true, d: true, l: true},
	'┬': {l: true, r: true, d: true}, '┴': {l: true, r: true, u: true},
	'┼': {u: true, d: true, l: true, r: true},
	'═': {l: true, r: true, double: true}, '║': {u: true, d: true, double: true},
	'╔': {r: true, d: true, double: true}, '╗': {l: true, d: true, double: true},
	'╚': {u: true, r: true, double: true}, '╝': {u: true, l: true, double: true},
	'╠': {u: true, d: true, r: true, double: true}, '╣': {u: true, d: true, l: true, double: true},
	'╦': {l: true, r: true, d: true, double: true}, '╩': {l: true, r: true, u: true, double: true},
	'╬': {u: true, d: true, l: true, r: true, double: true},
	'╒': {r: true, d: true}, '╕': {l: true, d: true}, '╘': {u: true, r: true}, '╛': {u: true, l: true},
	'╥': {l: true, r: true, d: true}, '╨': {l: true, r: true, u: true},
}

// addGlow blurs a coverage mask and adds it to the pixel layer as light.
func addGlow(p *Pixels, cov []float32, bw, bh, x0, y0, radius int, c RGB, strength float64) {
	blur := boxBlur(cov, bw, bh, radius)
	for yy := 0; yy < bh; yy++ {
		for xx := 0; xx < bw; xx++ {
			if v := blur[yy*bw+xx]; v > 0.003 {
				p.Add(x0+xx, y0+yy, c, float64(v)*strength)
			}
		}
	}
}

// FitBlock picks a font and scale at which s, wrapped to at most maxLines
// lines with gap blank rows between them, is as big as possible inside a
// maxW×maxH pixel box. Fonts missing a character of s are skipped. Fonts
// are listed in order of preference: a later font only wins if its letters
// come out clearly (15%+) taller. Scales snap to half pixels (whole pixels
// from 4 up), for nearly crisp edges.
func FitBlock(s string, maxW, maxH float64, maxLines, gap int, fonts ...*FigFont) (*FigFont, []string, float64) {
	names := make([]string, len(fonts))
	for i, f := range fonts {
		names[i] = f.Name
	}
	key := fmt.Sprintf("%q|%.1f|%.1f|%d|%d|%s", s, maxW, maxH, maxLines, gap, strings.Join(names, ","))
	if v, ok := fitBlockMemo.Load(key); ok {
		r := v.(fitBlockResult)
		return r.f, r.lines, r.scale
	}
	type cand struct {
		f      *FigFont
		lines  []string
		scale  float64
		letter float64
	}
	var cands []cand
	for _, f := range fonts {
		text := f.DropQuotes(s)
		if !f.Has(text) {
			continue
		}
		best := cand{f: f}
		bestWidest := 0
		full := 0
		for _, para := range strings.Split(text, "\n") {
			full = max(full, f.Width(para))
		}
		// Try narrower and narrower wraps; keep the one that scales up most.
		for tw := full; tw > 0; tw = tw * 94 / 100 {
			lines := balancedWrap(f, text, tw)
			if maxLines > 0 && len(lines) > maxLines {
				break
			}
			widest := 0
			for _, l := range lines {
				widest = max(widest, f.Width(l))
			}
			rows := len(lines)*f.Rows() + (len(lines)-1)*gap
			sc := snapScale(math.Min(maxW/float64(widest), maxH/(2*float64(rows))))
			// At the same scale, prefer fewer lines, then the narrower one.
			if sc > best.scale || (sc == best.scale && (len(lines) < len(best.lines) ||
				len(lines) == len(best.lines) && widest < bestWidest)) {
				best.lines, best.scale, bestWidest = lines, sc, widest
			}
			if tw < 8 {
				break
			}
		}
		if best.lines != nil {
			best.letter = best.scale * 2 * float64(f.Rows())
			cands = append(cands, best)
		}
	}
	var r fitBlockResult
	if len(cands) == 0 {
		f := fonts[len(fonts)-1]
		r = fitBlockResult{f, f.Wrap(f.DropQuotes(s), max(int(maxW), 1)), 0.5}
	} else {
		top := 0.0
		for _, c := range cands {
			top = math.Max(top, c.letter)
		}
		for _, c := range cands {
			if c.letter >= top/1.15 {
				r = fitBlockResult{c.f, c.lines, c.scale}
				break
			}
		}
	}
	fitBlockMemo.Store(key, r)
	return r.f, r.lines, r.scale
}

// balancedWrap wraps each paragraph of s to at most maxW cells in as few
// lines as greedy wrapping needs, but with the breaks evened out: it finds
// the narrowest width that still takes that many lines. "What Your MCP
// Server / Does" becomes "What Your MCP / Server Does".
func balancedWrap(f *FigFont, s string, maxW int) []string {
	var out []string
	for _, para := range strings.Split(s, "\n") {
		n := len(f.Wrap(para, maxW))
		lo, hi := 1, maxW // narrowest width that keeps n lines is in [lo, hi]
		for lo < hi {
			mid := (lo + hi) / 2
			if len(f.Wrap(para, mid)) <= n {
				hi = mid
			} else {
				lo = mid + 1
			}
		}
		out = append(out, f.Wrap(para, hi)...)
	}
	return out
}

type fitBlockResult struct {
	f     *FigFont
	lines []string
	scale float64
}

var fitBlockMemo sync.Map

// snapScale rounds a scale down to half pixels (whole pixels from 4 up), so
// block edges land on or halfway between pixels and stay nearly crisp.
func snapScale(s float64) float64 {
	if s >= 4 {
		return math.Floor(s)
	}
	return math.Max(math.Floor(s*2)/2, 0.5)
}
