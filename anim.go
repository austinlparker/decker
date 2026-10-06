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

// EaseInQuad accelerates from rest; p is clamped to 0..1.
func EaseInQuad(p float64) float64 { p = Clamp01(p); return p * p }

// EaseOutQuad decelerates, more gently than EaseOutCubic; p is clamped to 0..1.
func EaseOutQuad(p float64) float64 { p = Clamp01(p); return 1 - (1-p)*(1-p) }

// EaseInOutQuad accelerates then decelerates, more gently than
// EaseInOutCubic; p is clamped to 0..1.
func EaseInOutQuad(p float64) float64 {
	p = Clamp01(p)
	if p < 0.5 {
		return 2 * p * p
	}
	return 1 - math.Pow(-2*p+2, 2)/2
}

// EaseInCubic accelerates from rest, the mirror of EaseOutCubic: for exits.
func EaseInCubic(p float64) float64 { p = Clamp01(p); return p * p * p }

// EaseOutExpo decelerates sharply: most of the travel happens at the start.
func EaseOutExpo(p float64) float64 {
	p = Clamp01(p)
	if p >= 1 {
		return 1
	}
	return 1 - math.Pow(2, -10*p)
}

// EaseInOutExpo is slow at both ends with a fast middle.
func EaseInOutExpo(p float64) float64 {
	p = Clamp01(p)
	switch {
	case p <= 0:
		return 0
	case p >= 1:
		return 1
	case p < 0.5:
		return math.Pow(2, 20*p-10) / 2
	}
	return (2 - math.Pow(2, -20*p+10)) / 2
}

// EaseOutElastic overshoots and rings like a spring before it settles on 1.
func EaseOutElastic(p float64) float64 {
	p = Clamp01(p)
	if p <= 0 || p >= 1 {
		return p
	}
	const c4 = 2 * math.Pi / 3
	return math.Pow(2, -10*p)*math.Sin((p*10-0.75)*c4) + 1
}

// EaseOutBounce lands on 1 and bounces back up a few times, like a dropped
// ball.
func EaseOutBounce(p float64) float64 {
	p = Clamp01(p)
	const n, d = 7.5625, 2.75
	switch {
	case p < 1/d:
		return n * p * p
	case p < 2/d:
		p -= 1.5 / d
		return n*p*p + 0.75
	case p < 2.5/d:
		p -= 2.25 / d
		return n*p*p + 0.9375
	}
	p -= 2.625 / d
	return n*p*p + 0.984375
}

// CubicBezier returns the easing of CSS's cubic-bezier(x1, y1, x2, y2): a
// curve from (0,0) to (1,1) through two control points, with x1 and x2 in
// 0..1. y may leave 0..1 to overshoot. CubicBezier(0.25, 0.1, 0.25, 1) is
// CSS's "ease". Build it once and keep the func: the returned function does no
// allocation.
func CubicBezier(x1, y1, x2, y2 float64) func(float64) float64 {
	x1, x2 = Clamp01(x1), Clamp01(x2)
	cx, cy := 3*x1, 3*y1
	bx, by := 3*(x2-x1)-cx, 3*(y2-y1)-cy
	ax, ay := 1-cx-bx, 1-cy-by
	x := func(t float64) float64 { return ((ax*t+bx)*t + cx) * t }
	dx := func(t float64) float64 { return (3*ax*t+2*bx)*t + cx }
	return func(p float64) float64 {
		p = Clamp01(p)
		if p <= 0 || p >= 1 {
			return p
		}
		// Newton from the straight-line guess converges in a few steps on
		// the usual curves; bisection takes over where the slope is too flat
		// to trust it.
		t := p
		for range 8 {
			e := x(t) - p
			if math.Abs(e) < 1e-7 {
				return ((ay*t+by)*t + cy) * t
			}
			d := dx(t)
			if math.Abs(d) < 1e-6 {
				break
			}
			t = Clamp01(t - e/d)
		}
		lo, hi := 0.0, 1.0
		t = p
		for range 40 {
			e := x(t) - p
			if math.Abs(e) < 1e-7 {
				break
			}
			if e > 0 {
				hi = t
			} else {
				lo = t
			}
			t = (lo + hi) / 2
		}
		return ((ay*t+by)*t + cy) * t
	}
}

// Lerp is a+(b-a)*p; unlike Mix, p is not clamped.
func Lerp(a, b, p float64) float64 { return a + (b-a)*p }

// LerpInt is Lerp rounded to the nearest integer.
func LerpInt(a, b int, p float64) int { return int(math.Round(Lerp(float64(a), float64(b), p))) }

// Spring returns the position at time t of a Harmonica spring that starts at
// from and is pulled toward to. freq sets speed (try 2-8); damping < 1 bounces,
// 1 is critically damped. The result depends only on t, which counts in whole
// 1/60 s steps; it stops at settling or after 10 seconds.
func Spring(from, to, t, freq, damping float64) float64 {
	const fps = 60
	n := int(t * fps)
	if n <= 0 {
		return from
	}
	if n >= 10*fps { // any spring worth using has settled by 10s
		return to
	}
	// Harmonica's step is the exact solution for any dt, so n steps of 1/60 s
	// are one step of n/60 s.
	dt := float64(n) * harmonica.FPS(fps)
	pos, vel := harmonica.NewSpring(dt, freq, damping).Update(from, 0, to)
	// Land exactly on `to` once settled: a 1e-7 residual can flip a rounded
	// position every frame.
	eps := max(math.Abs(to-from), 1) * 1e-4
	if math.Abs(pos-to) < eps && math.Abs(vel) < eps {
		return to
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
	// Plain comparisons: per-pixel hot path, and math.Max/Min's NaN handling is
	// slower.
	if p < 0 {
		return 0
	}
	if p > 1 {
		return 1
	}
	return p
}
