package decker

import "math"

// GlyphFX is an effect's instruction for one glyph of Text.
type GlyphFX struct {
	DX, DY float64 // offset in pixels
	Alpha  float64 // 0 hides the glyph, 1 leaves it untouched
	Rune   rune    // 0 = the real character; ignored for spaces
}

// then combines b onto a: offsets add, alphas multiply, and a nonzero Rune
// replaces the earlier one.
func (a GlyphFX) then(b GlyphFX) GlyphFX {
	a.DX += b.DX
	a.DY += b.DY
	a.Alpha *= b.Alpha
	if b.Rune != 0 {
		a.Rune = b.Rune
	}
	return a
}

// GlyphEffect animates the glyphs of a Text (Text.FX). i counts glyphs
// across all lines, spaces included, plus one per line break.
//
// Every constructor below takes t, the seconds since the effect starts, and
// returns a GlyphEffect. To write one, follow that shape: the effect is a
// pure function of t and i, with no clock and no math/rand (use Hash01 for
// noise) so frames replay exactly. The zero GlyphFX hides the glyph, so
// return it before the effect has reached i and GlyphFX{Alpha: 1} for a
// glyph the effect leaves alone. Combine effects with Chain.
type GlyphEffect func(i int) GlyphFX

// staggered delays an effect by per seconds for each glyph: f gets the
// glyph's own time, and the glyph stays hidden until that is positive.
func staggered(t, per float64, f func(lt float64) GlyphFX) GlyphEffect {
	return func(i int) GlyphFX {
		if lt := t - float64(i)*per; lt > 0 {
			return f(lt)
		}
		return GlyphFX{}
	}
}

// RiseIn springs letters up from below and fades them in, one after another
// stagger seconds apart. size is the text size, which sets how far they
// travel.
func RiseIn(t, stagger float64, size int) GlyphEffect {
	return staggered(t, stagger, func(lt float64) GlyphFX {
		return GlyphFX{DY: Spring(1, 0, lt, 7, 0.55) * float64(size) * 0.7, Alpha: Ease(lt, 0.25)}
	})
}

// DropIn drops letters from above with a bounce, one after another stagger
// seconds apart.
func DropIn(t, stagger float64, size int) GlyphEffect {
	return staggered(t, stagger, func(lt float64) GlyphFX {
		return GlyphFX{DY: Spring(-1, 0, lt, 6, 0.4) * float64(size), Alpha: Ease(lt, 0.15)}
	})
}

// Decode cycles each letter through random characters before locking it in,
// left to right, like a terminal decrypting a message. The whole reveal
// takes dur seconds; n is the number of glyphs in the text.
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

// FadeUp fades the whole block in over dur seconds while it slides up
// 0.4×size.
func FadeUp(t, dur float64, size int) GlyphEffect {
	p := Ease(t, dur)
	return func(int) GlyphFX { return GlyphFX{DY: (1 - p) * float64(size) * 0.4, Alpha: p} }
}

// Wave bobs letters up and down by amp pixels in a travelling wave, for
// playful emphasis. It never settles.
func Wave(t, amp float64) GlyphEffect {
	return func(i int) GlyphFX {
		return GlyphFX{DY: amp * math.Sin(t*5-float64(i)*0.55), Alpha: 1}
	}
}

// Jitter shakes letters nervously by up to amp pixels (for "this is bad"
// moments).
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

// sweepPos is where a highlight band sits t seconds into a dur-second
// left-to-right sweep, starting and ending just off the block.
func sweepPos(t, dur float64) float64 { return Lerp(-0.3, 1.3, Progress(t, 0, dur)) }

// bandFalloff is the 0..1 strength of a highlight band at distance delta
// from its center, in block widths.
func bandFalloff(delta float64) float64 {
	d := math.Abs(delta) / 0.12
	if d >= 1 {
		return 0
	}
	return 1 - d*d
}

// ShineBand returns a Text.Shine func: a bright band that sweeps left to
// right once, starting at t=0 and taking dur seconds.
func ShineBand(t, dur, strength float64) func(float64) float64 {
	pos := sweepPos(t, dur)
	return func(u float64) float64 { return strength * bandFalloff(u-pos) }
}
