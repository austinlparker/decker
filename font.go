package decker

import (
	"embed"
	"io/fs"
	"math"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

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
	return &Font{otf: f, faces: map[int]font.Face{}, glyphs: map[glyphKey]*glyph{}, kerns: map[kernKey]float64{}}, nil
}

// Font is a typeface that draws raster type at any pixel size, with
// per-size caches of rasterized glyphs. Sizes are in pixels; the pixel
// layer is twice as tall as the terminal in cells, so a 40px font is about
// 20 terminal rows tall. Text draws with one.
type Font struct {
	otf    *opentype.Font
	mu     sync.Mutex // guards the caches; a font.Face is not safe for concurrent use
	faces  map[int]font.Face
	glyphs map[glyphKey]*glyph
	kerns  map[kernKey]float64

	// Small, if set, is used instead below SmallBelow pixels: a medium
	// weight gets spindly at low resolution, so point it at a bolder cut
	// (SpaceGrotesk-Bold for SpaceGrotesk-Medium).
	Small      *Font
	SmallBelow int
}

// minFontSize is the smallest size text is drawn at.
const minFontSize = 4

type glyphKey struct {
	r    rune
	size int
	q    int // subpixel offset in quarter pixels (0-3)
}

type kernKey struct {
	a, b rune
	size int
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

func unfix(v fixed.Int26_6) float64 { return float64(v) / 64 }

// face returns the cached face for size. f.mu must be held.
func (f *Font) face(size int) font.Face {
	size = max(size, minFontSize)
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

func (f *Font) glyph(r rune, size int) *glyph { return f.glyphAt(r, size, 0) }

// glyphAt returns r rendered with the pen shifted right by q/subpixel of a
// pixel.
func (f *Font) glyphAt(r rune, size, q int) *glyph {
	f.mu.Lock()
	defer f.mu.Unlock()
	k := glyphKey{r, size, q}
	if g, ok := f.glyphs[k]; ok {
		return g
	}
	dot := fixed.Point26_6{X: fixed.Int26_6(q * 64 / subpixel)}
	dr, mask, mp, adv, ok := f.face(size).Glyph(dot, r)
	g := &glyph{adv: unfix(adv), ox: dr.Min.X, oy: dr.Min.Y, w: dr.Dx(), h: dr.Dy()}
	if ok && g.w > 0 && g.h > 0 {
		g.a = make([]uint8, g.w*g.h)
		for y := range g.h {
			for x := range g.w {
				_, _, _, a := mask.At(mp.X+x, mp.Y+y).RGBA()
				g.a[y*g.w+x] = gammaLUT[uint8(a>>8)]
			}
		}
	}
	f.glyphs[k] = g
	return g
}

func (f *Font) kern(a, b rune, size int) float64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	k := kernKey{a, b, size}
	if v, ok := f.kerns[k]; ok {
		return v
	}
	v := unfix(f.face(size).Kern(a, b))
	f.kerns[k] = v
	return v
}

func (f *Font) metrics(size int) font.Metrics {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.face(size).Metrics()
}

// resolve returns the font and pixel size that text requested at size is
// drawn with: sizes below minFontSize are raised to it, and below
// SmallBelow the Small font replaces f, where thin strokes break up.
func (f *Font) resolve(size int) (*Font, int) {
	size = max(size, minFontSize)
	if f.Small != nil && size < f.SmallBelow {
		return f.Small, size
	}
	return f, size
}

// Drawn returns the size text requested at size is actually drawn at.
func (f *Font) Drawn(size int) int { return max(size, minFontSize) }

// Ascent is the distance from the top of a line to its baseline.
func (f *Font) Ascent(size int) float64 { return unfix(f.metrics(size).Ascent) }

// CapHeight is the height of capital letters.
func (f *Font) CapHeight(size int) float64 { return unfix(f.metrics(size).CapHeight) }

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

// Ink returns how far s's glyphs actually reach above and below the
// baseline at size: top is negative (above), bottom positive. Spaces don't
// count. Both are 0 if s has no visible glyphs.
func (f *Font) Ink(s string, size int) (top, bottom float64) {
	f, size = f.resolve(size)
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
