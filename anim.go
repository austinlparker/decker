package decker

import (
	"math"

	"github.com/charmbracelet/harmonica"
)

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

// Ease is EaseOutCubic(Progress(t, 0, dur)): the workhorse for entrances.
func Ease(t, dur float64) float64 { return EaseOutCubic(Progress(t, 0, dur)) }

// EaseOutCubic decelerates; p is clamped to 0..1.
func EaseOutCubic(p float64) float64 { p = Clamp01(p); return 1 - math.Pow(1-p, 3) }

// EaseInOutCubic accelerates then decelerates; transitions use it.
func EaseInOutCubic(p float64) float64 {
	p = Clamp01(p)
	if p < 0.5 {
		return 4 * p * p * p
	}
	return 1 - math.Pow(-2*p+2, 3)/2
}

// EaseOutBack overshoots slightly before settling, for "pop" entrances.
func EaseOutBack(p float64) float64 {
	p = Clamp01(p)
	const c1 = 1.70158
	const c3 = c1 + 1
	return 1 + c3*math.Pow(p-1, 3) + c1*math.Pow(p-1, 2)
}

// Lerp is a+(b-a)*p; unlike Mix, p is not clamped.
func Lerp(a, b, p float64) float64 { return a + (b-a)*p }

// LerpInt is Lerp rounded to the nearest integer.
func LerpInt(a, b int, p float64) int { return int(math.Round(Lerp(float64(a), float64(b), p))) }

// Spring returns the position at time t of a Harmonica spring that starts at
// from and is pulled toward to. freq sets speed (try 2-8); damping < 1 bounces,
// 1 is critically damped. Every call simulates from t=0, so the result depends
// only on t; it stops at settling or after 10 seconds.
func Spring(from, to, t, freq, damping float64) float64 {
	const fps = 60
	if t <= 0 {
		return from
	}
	n := int(t * fps)
	if n >= 10*fps { // any spring worth using has settled by 10s
		return to
	}
	sp := harmonica.NewSpring(harmonica.FPS(fps), freq, damping)
	pos, vel := from, 0.0
	// Land exactly on `to` once settled: a 1e-7 residual can flip a rounded position every frame.
	eps := max(math.Abs(to-from), 1) * 1e-4
	for i := 0; i < n; i++ {
		pos, vel = sp.Update(pos, vel, to)
		if math.Abs(pos-to) < eps && math.Abs(vel) < eps {
			return to
		}
	}
	return pos
}

// Pulse oscillates smoothly between 0 and 1 with the given period.
func Pulse(t, period float64) float64 { return 0.5 - 0.5*math.Cos(2*math.Pi*t/period) }

// Hash01 is a deterministic pseudo-random number in [0,1) for a cell. Use it
// instead of math/rand so every frame replays exactly.
func Hash01(x, y, seed int) float64 {
	h := uint32(x)*374761393 + uint32(y)*668265263 + uint32(seed)*2147483647
	h = (h ^ (h >> 13)) * 1274126177
	h ^= h >> 16
	return float64(h%10000) / 10000
}

// Clamp01 clamps p to [0, 1].
func Clamp01(p float64) float64 {
	// Plain comparisons: per-pixel hot path, and math.Max/Min's NaN handling is slower.
	if p < 0 {
		return 0
	}
	if p > 1 {
		return 1
	}
	return p
}
