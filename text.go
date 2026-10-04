package decker

import (
	"embed"
	"io/fs"
	"math"
	"strings"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// Raster type: real fonts drawn into the pixel layer at any size, so text
// can be as big as the stage needs regardless of the terminal's font size.
// Sizes are in pixels; the pixel layer is twice as tall as the terminal in
// cells, so a 40px font is about 20 terminal rows tall.

//go:embed fonts/*.ttf
var stockFonts embed.FS

// StockFont loads one of the typefaces that ship with the engine (SIL Open
// Font License; see fonts/): "SpaceGrotesk-Bold", "SpaceGrotesk-Medium" or
// "JetBrainsMono-ExtraBold". Load each once, at startup: a Font caches its
// rendered glyphs.
func StockFont(name string) *Font { return LoadFont(stockFonts, "fonts/"+name+".ttf") }

// LoadFont loads a TrueType or OpenType font from fsys, typically a deck's
// own embedded files. It panics if the font is missing or broken: fonts
// are built in, so that's a bug in the deck, not a runtime condition.
func LoadFont(fsys fs.FS, path string) *Font {
	b, err := fs.ReadFile(fsys, path)
	if err != nil {
		panic(err)
	}
	f, err := ParseFont(b)
	if err != nil {
		panic(path + ": " + err.Error())
	}
	return f
}

// ParseFont parses a TrueType or OpenType font.
func ParseFont(data []byte) (*Font, error) {
	f, err := opentype.Parse(data)
	if err != nil {
		return nil, err
	}
	return &Font{otf: f, faces: map[int]font.Face{}, glyphs: map[glyphKey]*glyph{}}, nil
}

// Font is a typeface with per-size caches of rasterized glyphs.
type Font struct {
	otf    *opentype.Font
	mu     sync.Mutex
	faces  map[int]font.Face
	glyphs map[glyphKey]*glyph
	kerns  map[kernKey]float64

	// Small, if set, is used instead below SmallBelow pixels: a medium
	// weight gets spindly at low resolution, so point it at a bolder cut.
	Small      *Font
	SmallBelow int
}

type glyphKey struct {
	r    rune
	size int
	q    int // subpixel offset in quarter pixels (0-3)
}

// subpixel is how many horizontal positions per pixel glyphs are rendered
// at. Without it, small type gets uneven letter spacing from rounding.
const subpixel = 4

// gammaLUT slightly thickens antialiased edges ("stem darkening") so light
// text on a dark background doesn't look thin and faint at small sizes.
var gammaLUT = func() (t [256]uint8) {
	for i := range t {
		t[i] = uint8(math.Round(255 * math.Pow(float64(i)/255, 0.75)))
	}
	return
}()

// glyph is a cached alpha mask. (ox, oy) is the mask's top-left corner
// relative to the pen position on the baseline.
type glyph struct {
	a      []uint8
	w, h   int
	ox, oy int
	adv    float64
}

func (f *Font) face(size int) font.Face {
	size = max(size, 4)
	if fc, ok := f.faces[size]; ok {
		return fc
	}
	fc, err := opentype.NewFace(f.otf, &opentype.FaceOptions{Size: float64(size), DPI: 72, Hinting: font.HintingNone})
	if err != nil {
		panic(err)
	}
	f.faces[size] = fc
	return fc
}

func (f *Font) glyph(r rune, size int) *glyph { return f.glyphQ(r, size, 0) }

// glyphQ returns r rendered with the pen shifted right by q/subpixel of a
// pixel.
func (f *Font) glyphQ(r rune, size, q int) *glyph {
	f.mu.Lock()
	defer f.mu.Unlock()
	k := glyphKey{r, size, q}
	if g, ok := f.glyphs[k]; ok {
		return g
	}
	fc := f.face(size)
	dot := fixed.Point26_6{X: fixed.Int26_6(q * 64 / subpixel)}
	dr, mask, mp, adv, ok := fc.Glyph(dot, r)
	g := &glyph{adv: float64(adv) / 64, ox: dr.Min.X, oy: dr.Min.Y, w: dr.Dx(), h: dr.Dy()}
	if ok && g.w > 0 && g.h > 0 {
		g.a = make([]uint8, g.w*g.h)
		for y := 0; y < g.h; y++ {
			for x := 0; x < g.w; x++ {
				_, _, _, a := mask.At(mp.X+x, mp.Y+y).RGBA()
				g.a[y*g.w+x] = gammaLUT[uint8(a>>8)]
			}
		}
	}
	f.glyphs[k] = g
	return g
}

// forSize returns the font to actually use at a size: faces with a Small
// fallback switch to it below SmallBelow pixels, where thin strokes break up.
func (f *Font) forSize(size int) *Font {
	if f.Small != nil && size < f.SmallBelow {
		return f.Small
	}
	return f
}

// resolve picks the face and size to actually draw with.
func (f *Font) resolve(size int) (*Font, int) {
	return f.forSize(size), max(size, 4)
}

// Drawn returns the size text requested at size is actually drawn at.
func (f *Font) Drawn(size int) int {
	_, s := f.resolve(size)
	return s
}

type kernKey struct {
	a, b rune
	size int
}

func (f *Font) kern(a, b rune, size int) float64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	k := kernKey{a, b, size}
	if v, ok := f.kerns[k]; ok {
		return v
	}
	if f.kerns == nil {
		f.kerns = map[kernKey]float64{}
	}
	v := float64(f.face(size).Kern(a, b)) / 64
	f.kerns[k] = v
	return v
}

// fitKey identifies a Fit/FitAll call; results are memoized because slides
// call them every frame with the same arguments.
type fitKey struct {
	f          *Font
	text       string
	maxW, maxH float64
	maxSize    int
	leading    float64
}

type fitResult struct {
	size  int
	lines []string
}

var (
	fitMu    sync.Mutex
	fitCache = map[fitKey]fitResult{}
)

func memoFit(k fitKey, compute func() (int, []string)) (int, []string) {
	fitMu.Lock()
	r, ok := fitCache[k]
	fitMu.Unlock()
	if ok {
		return r.size, r.lines
	}
	size, lines := compute()
	fitMu.Lock()
	if len(fitCache) > 5000 { // resizing the window creates new keys; keep it bounded
		fitCache = map[fitKey]fitResult{}
	}
	fitCache[k] = fitResult{size, lines}
	fitMu.Unlock()
	return size, lines
}

// Ascent is the distance from the top of a line to its baseline.
func (f *Font) Ascent(size int) float64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return float64(f.face(size).Metrics().Ascent) / 64
}

// CapHeight is the height of capital letters.
func (f *Font) CapHeight(size int) float64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return float64(f.face(size).Metrics().CapHeight) / 64
}

// Measure returns the width of a single line of text.
func (f *Font) Measure(s string, size int) float64 {
	f, size = f.resolve(size)
	w, prev := 0.0, rune(-1)
	for _, r := range s {
		if prev >= 0 {
			w += f.kern(prev, r, size)
		}
		w += f.glyph(r, size).adv
		prev = r
	}
	return w
}

// Wrap breaks s into lines no wider than maxW, at spaces. Explicit "\n"
// line breaks are kept. Lines are balanced: it uses the narrowest width that
// needs no more lines than maxW does, so you get "What Your MCP / Server
// Does" rather than "What Your MCP Server / Does".
func (f *Font) Wrap(s string, size int, maxW float64) []string {
	// Slides lay out every frame, and balancing takes ~25 greedy wraps,
	// so results are remembered.
	key := wrapKey{f, s, size, maxW}
	wrapMu.Lock()
	lines, ok := wrapCache[key]
	wrapMu.Unlock()
	if !ok {
		lines = f.wrapBalanced(s, size, maxW)
		wrapMu.Lock()
		if len(wrapCache) > 8192 {
			clear(wrapCache)
		}
		wrapCache[key] = lines
		wrapMu.Unlock()
	}
	return append([]string(nil), lines...)
}

type wrapKey struct {
	f    *Font
	s    string
	size int
	maxW float64
}

var (
	wrapMu    sync.Mutex
	wrapCache = map[wrapKey][]string{}
)

func (f *Font) wrapBalanced(s string, size int, maxW float64) []string {
	lines := f.wrapGreedy(s, size, maxW)
	lo, hi := maxW*0.4, maxW
	for i := 0; i < 12; i++ {
		mid := (lo + hi) / 2
		if len(f.wrapGreedy(s, size, mid)) <= len(lines) && fits(f, f.wrapGreedy(s, size, mid), size, mid) {
			hi = mid
		} else {
			lo = mid
		}
	}
	if b := f.wrapGreedy(s, size, hi); len(b) == len(lines) {
		return b
	}
	return lines
}

func fits(f *Font, lines []string, size int, maxW float64) bool {
	for _, l := range lines {
		if f.Measure(l, size) > maxW {
			return false
		}
	}
	return true
}

func (f *Font) wrapGreedy(s string, size int, maxW float64) []string {
	var out []string
	for _, para := range strings.Split(s, "\n") {
		line := ""
		for _, word := range strings.Fields(para) {
			try := word
			if line != "" {
				try = line + " " + word
			}
			if line != "" && f.Measure(try, size) > maxW {
				out = append(out, line)
				line = word
			} else {
				line = try
			}
		}
		out = append(out, line)
	}
	return out
}

// Fit finds the largest size (at most maxSize) at which s, wrapped to maxW,
// fits in maxW×maxH, and returns it with the wrapped text.
func (f *Font) Fit(s string, maxW, maxH float64, maxSize int, leading float64) (int, string) {
	if leading == 0 {
		leading = DefaultLeading
	}
	size, lines := memoFit(fitKey{f, s, maxW, maxH, maxSize, leading}, func() (int, []string) {
		for size := maxSize; size > 6; size-- {
			lines := f.Wrap(s, size, maxW)
			if fits(f, lines, size, maxW) && float64(len(lines))*float64(size)*leading <= maxH {
				return size, lines
			}
		}
		// Nothing fits the box: use the smallest size, wrapped to the width,
		// and let it run taller rather than off the side of the screen.
		_, smallest := f.resolve(6)
		return smallest, f.Wrap(s, smallest, maxW)
	})
	return size, strings.Join(lines, "\n")
}

// FitAll finds the largest size at which every part, each wrapped to maxW,
// fits together in maxW×maxH.
func FitAll(f *Font, parts []string, maxW, maxH float64, maxSize int) (int, []string) {
	return memoFit(fitKey{f, strings.Join(parts, "\x00"), maxW, maxH, maxSize, -1}, func() (int, []string) {
		for size := maxSize; size > 6; size-- {
			var lines []string
			ok := true
			for _, part := range parts {
				for _, l := range f.Wrap(part, size, maxW) {
					ok = ok && f.Measure(l, size) <= maxW
					lines = append(lines, l)
				}
			}
			if ok && float64(len(lines))*float64(size)*DefaultLeading <= maxH {
				return size, lines
			}
		}
		_, smallest := f.resolve(6)
		var lines []string
		for _, part := range parts {
			lines = append(lines, f.Wrap(part, smallest, maxW)...)
		}
		return smallest, lines
	})
}

// Align positions text relative to the x coordinate passed to Draw.
type Align int

const (
	Left   Align = iota // x is the left edge
	Center              // x is the center
	Right               // x is the right edge
)

// DefaultLeading is the line height, as a multiple of the text size, that
// Text uses when Leading is zero.
const DefaultLeading = 1.1

// GlyphFX animates one glyph: offset, opacity, and optionally a stand-in
// character (for scramble effects). See effects.go for ready-made ones.
type GlyphFX struct {
	DX, DY float64
	Alpha  float64
	Rune   rune // 0 = the real character
}

// Text describes how to draw a block of raster type.
type Text struct {
	Font    *Font   // required
	Size    int     // pixels
	Color   RGB     // fill color
	To      *RGB    // if set, a left-to-right gradient from Color to To
	Align   Align   // how x is interpreted
	Leading float64 // line height as a multiple of Size (default 1.1)

	Glow      float64 // soft glow strength, 0 (none) to ~1.5; pixel type gets much less
	GlowColor *RGB    // defaults to Color

	// MaxW, if set, wraps lines to this width so text never runs off the
	// side of its area.
	MaxW float64

	// Shine, if set, returns extra brightness (0..1) for a horizontal
	// position u from 0 (left edge) to 1 (right edge) of the block.
	// ShineBand makes a moving highlight.
	Shine func(u float64) float64

	// FX, if set, animates each glyph. i counts glyphs across all lines
	// (spaces included).
	FX func(i int) GlyphFX
}

// Ink returns how far s's glyphs actually reach above and below the
// baseline at size: top is negative (above), bottom positive. Spaces don't
// count. Both are 0 if s has no visible glyphs.
func (f *Font) Ink(s string, size int) (top, bottom float64) {
	f, size = f.resolve(max(size, 4))
	first := true
	for _, r := range s {
		g := f.glyph(r, size)
		if g.h == 0 {
			continue
		}
		t, b := float64(g.oy), float64(g.oy+g.h)
		if first || t < top {
			top = t
		}
		if first || b > bottom {
			bottom = b
		}
		first = false
	}
	return top, bottom
}

// DrawMid draws a single line of s so that its ink is centered vertically
// on cy (x as for Draw). Use it to put a label in the middle of a box:
// centering the line box instead leaves lowercase text sitting low, and
// descenders poking into the border.
func (t Text) DrawMid(p *Pixels, s string, x, cy float64) (w, h float64) {
	rf, size := t.font().resolve(max(t.Size, 4))
	top, bot := rf.Ink(s, size)
	if top == bot {
		top, bot = -rf.CapHeight(size), 0
	}
	baseline := math.Round(cy - (top+bot)/2)
	return t.Draw(p, s, x, baseline-t.baseOff(rf, size))
}

func (t Text) font() *Font {
	if t.Font == nil {
		panic("decker.Text: Font is required")
	}
	return t.Font
}

// Baseline is the distance from the top of a line to its baseline, as Draw
// lays lines out.
func (t Text) Baseline() float64 {
	f, size := t.font().resolve(max(t.Size, 4))
	return t.baseOff(f, size)
}

// baseOff is Baseline for a resolved font and size.
func (t Text) baseOff(f *Font, size int) float64 {
	lead := t.Leading
	if lead == 0 {
		lead = DefaultLeading
	}
	return f.Ascent(size) + (float64(size)*lead-float64(size))/2 - float64(size)*0.08
}

// Draw renders s (which may contain "\n") with its top edge at y and
// returns the block's width and height.
func (t Text) Draw(p *Pixels, s string, x, y float64) (w, h float64) {
	f, size := t.font().resolve(max(t.Size, 4))
	if t.MaxW > 0 {
		s = strings.Join(f.Wrap(s, size, t.MaxW), "\n")
	}
	lead := t.Leading
	if lead == 0 {
		lead = DefaultLeading
	}
	lines := strings.Split(s, "\n")
	lineH := float64(size) * lead
	// Center the ascender-to-descender box within each line.
	baseOff := t.baseOff(f, size)

	widths := make([]float64, len(lines))
	for i, l := range lines {
		widths[i] = f.Measure(l, size)
		w = math.Max(w, widths[i])
	}
	h = lineH * float64(len(lines))
	blockX := x
	switch t.Align {
	case Center:
		blockX = x - w/2
	case Right:
		blockX = x - w
	}

	// Rasterize into a coverage buffer covering the block plus room for
	// glow and effect motion.
	pad := float64(size)
	bx0 := int(math.Floor(blockX - pad))
	by0 := int(math.Floor(y - pad))
	bw := int(math.Ceil(w+2*pad)) + 1
	bh := int(math.Ceil(h+2*pad)) + 1
	cov := make([]float32, bw*bh)

	gi := 0
	for li, line := range lines {
		lx := blockX
		switch t.Align {
		case Center:
			lx = x - widths[li]/2
		case Right:
			lx = x - widths[li]
		}
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
			gx := lx + pen + fx.DX - float64(bx0)
			show := r
			if fx.Rune != 0 && r != ' ' {
				// Center the stand-in glyph in the real glyph's advance.
				show = fx.Rune
				gx += (g.adv - f.glyph(show, size).adv) / 2
			}
			ix := math.Floor(gx)
			q := int((gx - ix) * subpixel)
			stampGlyph(cov, bw, bh, f.glyphQ(show, size, q), ix, base+fx.DY-float64(by0), fx.Alpha)
			pen += g.adv
			prev = r
			gi++
		}
		gi++ // count the line break so effects flow across lines
	}

	glow := t.Glow
	if glow > 0 {
		gc := t.Color
		if t.GlowColor != nil {
			gc = *t.GlowColor
		}
		blur := boxBlur(cov, bw, bh, max(size/6, 2))
		for yy := 0; yy < bh; yy++ {
			for xx := 0; xx < bw; xx++ {
				if v := blur[yy*bw+xx]; v > 0.003 {
					p.Add(bx0+xx, by0+yy, gc, float64(v)*glow*0.9)
				}
			}
		}
	}

	for yy := 0; yy < bh; yy++ {
		for xx := 0; xx < bw; xx++ {
			a := cov[yy*bw+xx]
			if a <= 0.002 {
				continue
			}
			px := bx0 + xx
			u := 0.0
			if w > 0 {
				u = Clamp01((float64(px) - blockX) / w)
			}
			c := t.Color
			if t.To != nil {
				c = Mix(t.Color, *t.To, u)
			}
			if t.Shine != nil {
				c = Mix(c, RGB{255, 255, 255}, t.Shine(u))
			}
			p.Blend(px, by0+yy, c, float64(a))
		}
	}
	return w, h
}

// stampGlyph adds a glyph's coverage at pen position (px, baseline py) in
// buffer coordinates, taking the max with what's there.
func stampGlyph(cov []float32, bw, bh int, g *glyph, px, py, alpha float64) {
	if g.a == nil || alpha <= 0 {
		return
	}
	ox := int(math.Round(px)) + g.ox
	oy := int(math.Round(py)) + g.oy
	al := float32(Clamp01(alpha)) / 255
	for y := 0; y < g.h; y++ {
		ty := oy + y
		if ty < 0 || ty >= bh {
			continue
		}
		for x := 0; x < g.w; x++ {
			tx := ox + x
			if tx < 0 || tx >= bw {
				continue
			}
			v := float32(g.a[y*g.w+x]) * al
			if i := ty*bw + tx; v > cov[i] {
				cov[i] = v
			}
		}
	}
}

// boxBlur approximates a Gaussian blur with two box-blur passes in each
// direction.
func boxBlur(src []float32, w, h, r int) []float32 {
	a := append([]float32(nil), src...)
	b := make([]float32, len(src))
	for pass := 0; pass < 2; pass++ {
		blur1D(a, b, w, h, r, true)
		blur1D(b, a, w, h, r, false)
	}
	return a
}

func blur1D(src, dst []float32, w, h, r int, horizontal bool) {
	n, m := w, h
	if !horizontal {
		n, m = h, w
	}
	idx := func(i, j int) int {
		if horizontal {
			return j*w + i
		}
		return i*w + j
	}
	norm := 1 / float32(2*r+1)
	for j := 0; j < m; j++ {
		var sum float32
		for i := -r; i <= r; i++ {
			if i >= 0 && i < n {
				sum += src[idx(i, j)]
			}
		}
		for i := 0; i < n; i++ {
			dst[idx(i, j)] = sum * norm
			if out := i - r; out >= 0 {
				sum -= src[idx(out, j)]
			}
			if in := i + r + 1; in < n {
				sum += src[idx(in, j)]
			}
		}
	}
}
