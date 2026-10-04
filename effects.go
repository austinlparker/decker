package decker

import (
	"math"
)

// Ready-made per-glyph animations for Text.FX. Each takes the time since
// the effect should start (seconds) and returns a func for Text.FX.

// RiseIn: letters spring up from below and fade in, one after another.
// size is the text size (sets how far they travel).
func RiseIn(t, stagger float64, size int) func(int) GlyphFX {
	return func(i int) GlyphFX {
		lt := t - float64(i)*stagger
		if lt <= 0 {
			return GlyphFX{}
		}
		y := Spring(1, 0, lt, 7, 0.55)
		return GlyphFX{DY: y * float64(size) * 0.7, Alpha: Ease(lt, 0.25)}
	}
}

// DropIn: letters fall from above with a bounce.
func DropIn(t, stagger float64, size int) func(int) GlyphFX {
	return func(i int) GlyphFX {
		lt := t - float64(i)*stagger
		if lt <= 0 {
			return GlyphFX{}
		}
		y := Spring(-1, 0, lt, 6, 0.4)
		return GlyphFX{DY: y * float64(size), Alpha: Ease(lt, 0.15)}
	}
}

// Decode: each letter cycles through random characters before locking in,
// left to right, like a terminal decrypting a message. dur is how long the
// whole reveal takes.
func Decode(t, dur float64, n int) func(int) GlyphFX {
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

// TypeOn: letters appear one at a time at cps characters per second.
func TypeOn(t, cps float64) func(int) GlyphFX {
	return func(i int) GlyphFX {
		if float64(i) < t*cps {
			return GlyphFX{Alpha: 1}
		}
		return GlyphFX{}
	}
}

// FadeUp: the whole block fades in while sliding up a little.
func FadeUp(t, dur float64, size int) func(int) GlyphFX {
	p := Ease(t, dur)
	return func(int) GlyphFX { return GlyphFX{DY: (1 - p) * float64(size) * 0.4, Alpha: p} }
}

// Wave: a continuous gentle bob, for playful emphasis.
func Wave(t, amp float64) func(int) GlyphFX {
	return func(i int) GlyphFX {
		return GlyphFX{DY: amp * math.Sin(t*5-float64(i)*0.55), Alpha: 1}
	}
}

// Jitter: letters shake nervously (for "this is bad" moments).
func Jitter(t, amp float64) func(int) GlyphFX {
	frame := int(t * 20)
	return func(i int) GlyphFX {
		return GlyphFX{
			DX:    amp * (Hash01(i, frame, 21) - 0.5) * 2,
			DY:    amp * (Hash01(i, frame, 22) - 0.5) * 2,
			Alpha: 1,
		}
	}
}

// Chain runs effects together (offsets add, alphas multiply).
func Chain(fxs ...func(int) GlyphFX) func(int) GlyphFX {
	return func(i int) GlyphFX {
		out := GlyphFX{Alpha: 1}
		for _, f := range fxs {
			g := f(i)
			out.DX += g.DX
			out.DY += g.DY
			out.Alpha *= g.Alpha
			if g.Rune != 0 {
				out.Rune = g.Rune
			}
		}
		return out
	}
}

// ShineBand returns a Text.Shine func: a bright diagonal-ish band that
// sweeps left to right once, starting at t=0 and taking dur seconds.
func ShineBand(t, dur, strength float64) func(float64) float64 {
	pos := Lerp(-0.3, 1.3, Progress(t, 0, dur))
	return func(u float64) float64 {
		d := math.Abs(u-pos) / 0.12
		if d >= 1 {
			return 0
		}
		return strength * (1 - d*d)
	}
}
