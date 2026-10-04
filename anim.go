package decker

import (
	"math"

	"github.com/charmbracelet/harmonica"
)

// Animation helpers. Everything here is a pure function of time, so slides
// stay replayable. Times are in seconds.

// Progress maps t onto 0..1 over [start, start+dur], clamped.
func Progress(t, start, dur float64) float64 {
	if dur <= 0 {
		if t >= start {
			return 1
		}
		return 0
	}
	return Clamp01((t - start) / dur)
}

// Ease is Progress from 0 with ease-out-cubic applied: fast start, gentle
// landing. The workhorse for entrances.
func Ease(t, dur float64) float64 { return EaseOutCubic(Progress(t, 0, dur)) }

func EaseOutCubic(p float64) float64 { p = Clamp01(p); return 1 - math.Pow(1-p, 3) }

func EaseInOutCubic(p float64) float64 {
	p = Clamp01(p)
	if p < 0.5 {
		return 4 * p * p * p
	}
	return 1 - math.Pow(-2*p+2, 3)/2
}

// EaseOutBack overshoots slightly before settling; good for "pop" entrances.
func EaseOutBack(p float64) float64 {
	p = Clamp01(p)
	const c1 = 1.70158
	const c3 = c1 + 1
	return 1 + c3*math.Pow(p-1, 3) + c1*math.Pow(p-1, 2)
}

// Lerp interpolates between a and b.
func Lerp(a, b, p float64) float64 { return a + (b-a)*p }

// LerpInt interpolates and rounds to the nearest cell.
func LerpInt(a, b int, p float64) int { return int(math.Round(Lerp(float64(a), float64(b), p))) }

// Spring returns the position of a Harmonica spring that started at `from`
// at t=0 and is pulled toward `to`. freq controls speed (try 2-8) and damping
// controls bounce (<1 bounces, 1 is critically damped).
//
// The spring is simulated from zero on every call so the result depends only
// on t. That's cheap: at most a few hundred steps.
func Spring(from, to, t, freq, damping float64) float64 {
	const fps = 60
	if t <= 0 {
		return from
	}
	sp := harmonica.NewSpring(harmonica.FPS(fps), freq, damping)
	pos, vel := from, 0.0
	n := min(int(t*fps), 10*fps) // any spring worth using has settled by 10s
	// Once it's settled, land exactly on `to`: a residual of 1e-7 can still
	// flip a pixel-rounded position back and forth every frame, and every
	// changed cell is work for the terminal.
	eps := math.Max(math.Abs(to-from), 1) * 1e-4
	for i := 0; i < n; i++ {
		pos, vel = sp.Update(pos, vel, to)
		if math.Abs(pos-to) < eps && math.Abs(vel) < eps {
			return to
		}
	}
	if n == 10*fps {
		return to
	}
	return pos
}

// Pulse oscillates smoothly between 0 and 1 with the given period.
func Pulse(t, period float64) float64 { return 0.5 - 0.5*math.Cos(2*math.Pi*t/period) }

// Hash01 is a cheap deterministic pseudo-random number in [0,1) for a cell,
// used for dissolves and sparkles without any global RNG state. Slides use
// it instead of math/rand so every frame can be replayed.
func Hash01(x, y, seed int) float64 {
	h := uint32(x)*374761393 + uint32(y)*668265263 + uint32(seed)*2147483647
	h = (h ^ (h >> 13)) * 1274126177
	h ^= h >> 16
	return float64(h%10000) / 10000
}

// Clamp01 clamps p to [0, 1].
func Clamp01(p float64) float64 {
	// Plain comparisons: this runs per pixel, and math.Max/Min's NaN
	// handling makes them several times slower.
	if p < 0 {
		return 0
	}
	if p > 1 {
		return 1
	}
	return p
}
