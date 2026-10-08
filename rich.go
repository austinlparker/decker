package decker

import (
	"encoding/binary"
	"math"
	"reflect"
	"strings"
	"unicode"
)

// Span is a run of text with its own style inside a Rich block. Its zero
// style inherits everything from the Rich.
type Span struct {
	Text      string
	Font      *Font // nil inherits Rich.Font
	Color     *RGB  // nil inherits Rich.Color
	Underline bool  // a rule under the glyphs, in the span's color
	Strike    bool  // a rule through the glyphs, in the span's color
	Mark      *RGB  // a highlighter-pen plate behind the glyphs; also how inline code gets its plate
}

// Rich draws a block of raster type whose spans differ in font, color and
// decoration; like Text it is a value to fill in and Draw. Sizes below 4 pixels
// are drawn at 4.
//
// A line breaks at "\n" inside a span and, with MaxW set, wraps at spaces
// across span boundaries (a word may start in one span and end in another).
// Every glyph on a line sits on the base font's baseline, so a Mono span
// shares its line with Body text without stepping up or down.
type Rich struct {
	Font    *Font   // required: the base face, and what sets the baseline
	Size    int     // pixels
	Color   RGB     // fill for spans without a Color
	Align   Align   // how x is interpreted
	Leading float64 // line height as a multiple of Size (default 1.1)
	MaxW    float64 // if set, wraps lines to this width

	// Glow is a soft glow strength, 0 (none) to ~1.5, in each span's color.
	Glow float64

	// FX animates glyphs. Indexes count every rune of every span as Text counts
	// them: spaces included, plus one per line break.
	FX GlyphEffect
}

func (r Rich) resolved() (*Font, int) {
	if r.Font == nil {
		panic("decker.Rich: Font is required")
	}
	return r.Font.resolve(r.Size)
}

// text is the Text that shares r's line metrics, so the two lay a baseline
// out the same way.
func (r Rich) text() Text { return Text{Font: r.Font, Size: r.Size, Leading: r.Leading} }

// Baseline is the distance from the top of a line to its baseline, as Draw
// lays it out.
func (r Rich) Baseline() float64 {
	f, size := r.resolved()
	return r.text().baseOff(f, size)
}

// Measure returns the size of the block Draw would draw for spans.
func (r Rich) Measure(spans []Span) (w, h float64) {
	_, size := r.resolved()
	l, _ := r.layout(spans)
	return l.w, float64(len(l.lines)) * float64(size) * leadingOr(r.Leading)
}

// richGlyph is one rune of the flattened spans, measured at the block's size.
type richGlyph struct {
	r    rune
	span int
	font *Font   // the span's font after Small substitution
	adv  float64 // advance
	kern float64 // kerning against the previous glyph, 0 if that is in another font
}

// richPlaced is a glyph placed on a line, pen being its offset from the
// line's left edge.
type richPlaced struct {
	g   int
	pen float64
}

// richLayout is the pure result of breaking spans into lines at one size and
// width. It depends on the text and fonts only, not on colors or decoration,
// so one layout serves every frame of a slide.
type richLayout struct {
	base         *Font // keeps the base font's address from being reused while cached
	glyphs       []richGlyph
	lines        [][]richPlaced
	widths       []float64
	w            float64
	inkTop, inkB float64 // reach of the glyphs above and below the baseline, as Font.Ink
}

// richBase is where the placeholder runes begin. Wrapping reuses the string
// wrapper by encoding each non-space glyph as one private-use rune, which no
// wrapper treats as a space; the layout maps them back to glyphs.
const richBase = 0x100000

type richKey struct {
	sig  string
	size int
	maxW float64
}

var richLayouts = memo[richKey, *richLayout]{max: 4096}

// richSig identifies spans by their text and fonts, the only inputs a layout
// reads. It allocates once, which is what lets a View lay out every frame.
func richSig(base *Font, spans []Span) string {
	n := 8
	for i := range spans {
		n += 8 + binary.MaxVarintLen64 + len(spans[i].Text)
	}
	b := make([]byte, 0, n)
	b = binary.LittleEndian.AppendUint64(b, fontID(base))
	for i := range spans {
		b = binary.LittleEndian.AppendUint64(b, fontID(spans[i].Font))
		b = binary.AppendUvarint(b, uint64(len(spans[i].Text)))
		b = append(b, spans[i].Text...)
	}
	return string(b)
}

func fontID(f *Font) uint64 {
	if f == nil {
		return 0
	}
	return uint64(reflect.ValueOf(f).Pointer())
}

// layout returns the memoized layout of spans, and its signature for callers
// that go on to look up more.
func (r Rich) layout(spans []Span) (*richLayout, string) {
	r.resolved() // panics if Font is nil
	sig := richSig(r.Font, spans)
	return r.layoutSig(sig, spans, r.Size), sig
}

func (r Rich) layoutSig(sig string, spans []Span, size int) *richLayout {
	return richLayouts.get(richKey{sig, size, max(r.MaxW, 0)}, func() *richLayout {
		return buildRichLayout(spans, r.Font, size, r.MaxW)
	})
}

func buildRichLayout(spans []Span, font *Font, size int, maxW float64) *richLayout {
	base, size := font.resolve(size)
	l := &richLayout{base: font}
	var enc strings.Builder
	first := true
	for si := range spans {
		f := base
		if spans[si].Font != nil {
			f, _ = spans[si].Font.resolve(size)
		}
		for _, r := range spans[si].Text {
			g := richGlyph{r: r, span: si, font: f}
			if r == '\n' {
				enc.WriteRune('\n')
			} else {
				gl := f.glyph(r, size)
				g.adv = gl.adv
				if n := len(l.glyphs); n > 0 && l.glyphs[n-1].r != '\n' && l.glyphs[n-1].font == f {
					g.kern = f.kern(l.glyphs[n-1].r, r, size)
				}
				if gl.h > 0 {
					t, b := float64(gl.oy), float64(gl.oy+gl.h)
					if first || t < l.inkTop {
						l.inkTop = t
					}
					if first || b > l.inkB {
						l.inkB = b
					}
					first = false
				}
				if unicode.IsSpace(r) {
					enc.WriteByte(' ')
				} else {
					enc.WriteRune(rune(richBase + len(l.glyphs)))
				}
			}
			l.glyphs = append(l.glyphs, g)
		}
	}

	var lines [][]int
	if maxW > 0 {
		for _, s := range wrapBalanced(enc.String(), maxW, l.measure) {
			line := []int{}
			l.eachGlyph(s, func(g int) { line = append(line, g) })
			lines = append(lines, line)
		}
	} else {
		// Unwrapped text keeps its spaces exactly, so indented code survives.
		line := []int{}
		for i, g := range l.glyphs {
			if g.r == '\n' {
				lines = append(lines, line)
				line = []int{}
			} else {
				line = append(line, i)
			}
		}
		lines = append(lines, line)
	}

	for _, idx := range lines {
		pen, prev := 0.0, -1
		line := make([]richPlaced, len(idx))
		for i, g := range idx {
			if prev >= 0 && prev == g-1 {
				pen += l.glyphs[g].kern
			}
			line[i] = richPlaced{g, pen}
			pen += l.glyphs[g].adv
			prev = g
		}
		l.lines = append(l.lines, line)
		l.widths = append(l.widths, pen)
		l.w = max(l.w, pen)
	}
	return l
}

// eachGlyph calls fn with the glyph index of each glyph of a wrapped line. A
// space between words stands for the original space just before the next word,
// so it keeps that space's span; the wrapper collapses runs of spaces to one,
// as Text's wrapping does.
func (l *richLayout) eachGlyph(line string, fn func(g int)) {
	space := false
	for _, r := range line {
		if r == ' ' {
			space = true
			continue
		}
		g := int(r) - richBase
		if space {
			fn(g - 1)
			space = false
		}
		fn(g)
	}
}

// measure is the width of a wrapped line, laid out as buildRichLayout lays it.
func (l *richLayout) measure(line string) float64 {
	w, prev := 0.0, -1
	l.eachGlyph(line, func(g int) {
		if prev >= 0 && prev == g-1 {
			w += l.glyphs[g].kern
		}
		w += l.glyphs[g].adv
		prev = g
	})
	return w
}

// Draw renders spans with their top edge at y and returns the block's size.
func (r Rich) Draw(p *Pixels, spans []Span, x, y float64) (w, h float64) {
	f, size := r.resolved()
	l, _ := r.layout(spans)
	lineH := float64(size) * leadingOr(r.Leading)
	w, h = l.w, lineH*float64(len(l.lines))
	blockX := x + r.Align.shift(w)

	// Each distinct color gets its own mask, painted marks first, then glows,
	// then ink, so a span's plate never covers its neighbor's letters.
	pad := float64(size)
	x0, y0 := int(math.Floor(blockX-pad)), int(math.Floor(y-pad))
	cw, ch := int(math.Ceil(w+2*pad))+1, int(math.Ceil(h+2*pad))+1
	var marks, inks richLayers
	baseOff := r.text().baseOff(f, size)
	fs := float64(size)
	gi := 0
	for li, line := range l.lines {
		lx := x + r.Align.shift(l.widths[li])
		base := y + float64(li)*lineH + baseOff
		for k, pg := range line {
			g := l.glyphs[pg.g]
			sp := &spans[g.span]
			fx := GlyphFX{Alpha: 1}
			if r.FX != nil {
				fx = r.FX(gi)
			}
			gi++
			col := r.Color
			if sp.Color != nil {
				col = *sp.Color
			}
			gx := lx + pg.pen + fx.DX - float64(x0)
			gy := base + fx.DY - float64(y0)
			if sp.Mark != nil {
				// The plate runs a little past the ink at the ends of a run.
				left, right := 0.5, 0.5
				if k == 0 || l.glyphs[line[k-1].g].span != g.span {
					left = fs * 0.12
				}
				if k == len(line)-1 || l.glyphs[line[k+1].g].span != g.span {
					right = fs * 0.12
				}
				marks.get(*sp.Mark, x0, y0, cw, ch).fillRect(gx-left, gy-fs*0.8, gx+g.adv+right, gy+fs*0.2, fx.Alpha)
			}
			in := inks.get(col, x0, y0, cw, ch)
			show := g.r
			if fx.Rune != 0 && g.r != ' ' {
				// Center the stand-in glyph in the real glyph's advance.
				show = fx.Rune
				gx += (g.adv - g.font.glyph(show, size).adv) / 2
			}
			ix := math.Floor(gx)
			q := int((gx - ix) * subpixel)
			in.stamp(g.font.glyphAt(show, size, q), ix, gy, fx.Alpha)
			if sp.Underline || sp.Strike {
				th := max(1, fs/16)
				if sp.Underline {
					in.fillRect(gx-0.5, gy+fs*0.1, gx+g.adv+0.5, gy+fs*0.1+th, fx.Alpha)
				}
				if sp.Strike {
					in.fillRect(gx-0.5, gy-fs*0.3-th/2, gx+g.adv+0.5, gy-fs*0.3+th/2, fx.Alpha)
				}
			}
		}
		gi++
	}

	for _, m := range marks {
		paintFlat(p, m.cov, m.col)
	}
	if r.Glow > 0 {
		for _, in := range inks {
			in.cov.addGlow(p, max(size/6, 2), in.col, r.Glow*0.9)
		}
	}
	for _, in := range inks {
		paintFlat(p, in.cov, in.col)
	}
	if p.review != nil {
		covs := make([]coverage, len(inks))
		var text strings.Builder
		for i, in := range inks {
			covs[i] = in.cov
		}
		for _, sp := range spans {
			text.WriteString(sp.Text)
		}
		boxes := make([]Rect, len(l.lines))
		for i := range l.lines {
			boxes[i] = Rect{x + r.Align.shift(l.widths[i]), y + float64(i)*lineH, l.widths[i], lineH}
		}
		checkInk(p, covs, f, size, text.String(), quoteText("Rich", text.String()), Rect{blockX, y, w, h}, boxes)
	}
	return w, h
}

// DrawMid draws spans as one line with its ink centered vertically on cy (x as
// for Draw), as Text.DrawMid does.
func (r Rich) DrawMid(p *Pixels, spans []Span, x, cy float64) (w, h float64) {
	f, size := r.resolved()
	l, _ := r.layout(spans)
	top, bot := l.inkTop, l.inkB
	if top == bot {
		top, bot = -f.CapHeight(size), 0
	}
	baseline := math.Round(cy - (top+bot)/2)
	return r.Draw(p, spans, x, baseline-r.text().baseOff(f, size))
}

// richLayer is a coverage mask painted in one color.
type richLayer struct {
	col RGB
	cov coverage
}

// richLayers is a short list: blocks use a handful of colors, so a linear
// search beats a map and keeps painting order deterministic.
type richLayers []richLayer

func (ls *richLayers) get(col RGB, x0, y0, w, h int) coverage {
	for _, l := range *ls {
		if l.col == col {
			return l.cov
		}
	}
	c := newCoverage(x0, y0, w, h)
	*ls = append(*ls, richLayer{col, c})
	return c
}

func paintFlat(p *Pixels, cov coverage, col RGB) {
	for y := 0; y < cov.h; y++ {
		for x := 0; x < cov.w; x++ {
			if a := cov.a[y*cov.w+x]; a > 0.002 {
				p.Blend(cov.x0+x, cov.y0+y, col, float64(a))
			}
		}
	}
}

type richFitKey struct {
	sig        string
	maxW, maxH float64
	maxSize    int
	leading    float64
}

var richFitted = memo[richFitKey, int]{max: 5000}

// Fit returns the largest size (at most maxSize) at which spans, wrapped to
// maxW, fit maxW×maxH, as Font.Fit does for plain text; set it as r.Size and
// r.MaxW = maxW to draw the result. It uses r.Font and r.Leading and ignores
// r.Size and r.MaxW. If nothing fits it returns the smallest size (wrapped to
// the width): running taller beats running off the side. Results are
// remembered, so a View can call it every frame.
func (r Rich) Fit(spans []Span, maxW, maxH float64, maxSize int) int {
	r.resolved()
	lead := leadingOr(r.Leading)
	sig := richSig(r.Font, spans)
	return richFitted.get(richFitKey{sig, maxW, maxH, maxSize, lead}, func() int {
		r.MaxW = maxW
		for size := maxSize; size > minFitSize; size-- {
			l := r.layoutSig(sig, spans, size)
			if l.w <= maxW && float64(len(l.lines))*float64(size)*lead <= maxH {
				return size
			}
		}
		return minFitSize
	})
}
