package decker

import "math"

// GlyphFX is an effect's instruction for one glyph of Text.
type GlyphFX struct {
	DX, DY float64 // offset in pixels
	Alpha  float64 // 0 hides the glyph, 1 leaves it untouched
	Rune   rune    // 0 = the real character; ignored for spaces
}

func (a GlyphFX) then(b GlyphFX) GlyphFX {
	a.DX += b.DX
	a.DY += b.DY
	a.Alpha *= b.Alpha
	if b.Rune != 0 {
		a.Rune = b.Rune
	}
	return a
}

// GlyphEffect animates the glyphs of a Text (Text.FX); i counts glyphs across all
// lines, spaces and line breaks included.
//
// Constructors take t, the seconds since the effect starts. Effects must be pure
// functions of t and i (no clock, no math/rand; use Hash01) so frames replay
// exactly. The zero GlyphFX hides the glyph: return it before the effect reaches
// i, and GlyphFX{Alpha: 1} for glyphs left alone.
type GlyphEffect func(i int) GlyphFX

func staggered(t, per float64, f func(lt float64) GlyphFX) GlyphEffect {
	return func(i int) GlyphFX {
		if lt := t - float64(i)*per; lt > 0 {
			return f(lt)
		}
		return GlyphFX{}
	}
}

// RiseIn springs letters up from below, fading in; size sets how far they travel.
func RiseIn(t, stagger float64, size int) GlyphEffect {
	return staggered(t, stagger, func(lt float64) GlyphFX {
		return GlyphFX{DY: Spring(1, 0, lt, 7, 0.55) * float64(size) * 0.7, Alpha: Ease(lt, 0.25)}
	})
}

// DropIn drops letters from above with a bounce, stagger seconds apart.
func DropIn(t, stagger float64, size int) GlyphEffect {
	return staggered(t, stagger, func(lt float64) GlyphFX {
		return GlyphFX{DY: Spring(-1, 0, lt, 6, 0.4) * float64(size), Alpha: Ease(lt, 0.15)}
	})
}

// Decode cycles letters through random characters before locking them in, left to
// right over dur seconds; n is the number of glyphs.
func Decode(t, dur float64, n int) GlyphEffect {
	const pool = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789#%&@$*+=<>/"
	return func(i int) GlyphFX {
		if t <= 0 {
			return GlyphFX{}
		}
		lock := dur * float64(i+1) / float64(max(n, 1))
		if t >= lock {
			return GlyphFX{Alpha: 1}
		}
		frame := int(t * 24)
		r := rune(pool[int(Hash01(i, frame, 11)*float64(len(pool)))])
		return GlyphFX{Alpha: 0.35 + 0.4*Hash01(i, frame, 12), Rune: r}
	}
}

// TypeOn shows letters one at a time at cps characters per second.
func TypeOn(t, cps float64) GlyphEffect {
	return func(i int) GlyphFX {
		if float64(i) < t*cps {
			return GlyphFX{Alpha: 1}
		}
		return GlyphFX{}
	}
}

// FadeUp fades the block in over dur seconds while sliding up 0.4×size.
func FadeUp(t, dur float64, size int) GlyphEffect {
	p := Ease(t, dur)
	return func(int) GlyphFX { return GlyphFX{DY: (1 - p) * float64(size) * 0.4, Alpha: p} }
}

// Wave bobs letters by amp pixels in a travelling wave. It never settles.
func Wave(t, amp float64) GlyphEffect {
	return func(i int) GlyphFX {
		return GlyphFX{DY: amp * math.Sin(t*5-float64(i)*0.55), Alpha: 1}
	}
}

// Jitter shakes letters by up to amp pixels.
func Jitter(t, amp float64) GlyphEffect {
	frame := int(t * 20)
	return func(i int) GlyphFX {
		return GlyphFX{
			DX:    amp * (Hash01(i, frame, 21) - 0.5) * 2,
			DY:    amp * (Hash01(i, frame, 22) - 0.5) * 2,
			Alpha: 1,
		}
	}
}

// Chain runs effects together: offsets add, alphas multiply, and the last
// stand-in rune wins.
func Chain(fxs ...GlyphEffect) GlyphEffect {
	return func(i int) GlyphFX {
		out := GlyphFX{Alpha: 1}
		for _, f := range fxs {
			out = out.then(f(i))
		}
		return out
	}
}

// sweepPos runs -0.3 to 1.3 over dur, so the band starts and ends off the block.
func sweepPos(t, dur float64) float64 { return Lerp(-0.3, 1.3, Progress(t, 0, dur)) }

func bandFalloff(delta float64) float64 {
	d := math.Abs(delta) / 0.12
	if d >= 1 {
		return 0
	}
	return 1 - d*d
}

// ShineBand returns a Text.Shine func: a bright band sweeping left to right once
// over dur seconds from t=0.
func ShineBand(t, dur, strength float64) func(float64) float64 {
	pos := sweepPos(t, dur)
	return func(u float64) float64 { return strength * bandFalloff(u-pos) }
}
