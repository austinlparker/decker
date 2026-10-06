package decker

import "math"

// Element animations are Composites that are pure functions of t, the seconds
// since the effect starts, like the letter effects. Entrances start hidden
// (and stay so for t < 0) and end at [Identity]; exits start at Identity (for
// t < 0 too) and end hidden; emphasis starts and ends at Identity. Join them
// with [Composite.Then] or [Combine], and draw with [Composite.Draw]:
//
//	fx := FlyIn(c, c.Since(1), 0.5, DirLeft, 0.1).Then(Grow(c.Since(2), 0.4, 1.1))
//	fx.Draw(p, r, func(p *Pixels) { Panel(c, p, r.X, r.Y, r.W, r.H, "hi", fill, edge, text, 1) })

// shown is Alpha in 0..1 as a Composite.
func shown(a float64) Composite {
	k := Identity()
	k.Alpha = a
	return k
}

// FadeIn fades an element in over dur seconds.
func FadeIn(t, dur float64) Composite {
	return shown(EaseInOutQuad(Progress(t, 0, dur)))
}

// FadeOut fades an element out over dur seconds.
func FadeOut(t, dur float64) Composite {
	return shown(1 - EaseInOutQuad(Progress(t, 0, dur)))
}

// offset is dist of the screen in direction d, in pixels: dist of its width
// for left and right, of its height for up and down.
func offset(c Ctx, d Direction, dist float64) (dx, dy float64) {
	switch d {
	case DirLeft:
		return -dist * c.PW(), 0
	case DirRight:
		return dist * c.PW(), 0
	case DirUp:
		return 0, -dist * c.PH()
	}
	return 0, dist * c.PH()
}

// FlyIn slides an element in over dur seconds from dist of the screen away in
// direction from (DirLeft starts it left of its place; DirDefault below it),
// fading in over the first half. dist 1 starts it just off the screen.
func FlyIn(c Ctx, t, dur float64, from Direction, dist float64) Composite {
	p := Progress(t, 0, dur)
	k := shown(Clamp01(2 * p))
	rest := 1 - EaseOutCubic(p)
	dx, dy := offset(c, from, dist)
	k.DX, k.DY = dx*rest, dy*rest
	return k
}

// FlyOut slides an element away over dur seconds, dist of the screen toward
// direction to, fading out over the second half.
func FlyOut(c Ctx, t, dur float64, to Direction, dist float64) Composite {
	p := Progress(t, 0, dur)
	k := shown(1 - Clamp01(2*p-1))
	gone := EaseInCubic(p)
	dx, dy := offset(c, to, dist)
	k.DX, k.DY = dx*gone, dy*gone
	return k
}

// minScale keeps a zoom from landing on Scale 0, which Composite reads as 1.
const minScale = 1e-3

// ZoomIn grows an element about its center from the given scale (try 0.6) to
// 1 over dur seconds, fading in over the first half.
func ZoomIn(t, dur, from float64) Composite {
	p := Progress(t, 0, dur)
	k := shown(Clamp01(2 * p))
	k.Scale = max(Lerp(from, 1, EaseOutCubic(p)), minScale)
	return k
}

// ZoomOut shrinks an element about its center from 1 to the given scale (try
// 0.6) over dur seconds, fading out over the second half.
func ZoomOut(t, dur, to float64) Composite {
	p := Progress(t, 0, dur)
	k := shown(1 - Clamp01(2*p-1))
	k.Scale = max(Lerp(1, to, EaseInCubic(p)), minScale)
	return k
}

// Pop grows an element from nothing about its center over dur seconds,
// overshooting its size a little with [EaseOutBack] before settling.
func Pop(t, dur float64) Composite {
	p := Progress(t, 0, dur)
	k := shown(Clamp01(6 * p))
	k.Scale = max(EaseOutBack(p), minScale)
	return k
}

// wipeTrim is the trim that leaves the fraction v of an element visible,
// anchored at its edge side.
func wipeTrim(edge Direction, v float64) Composite {
	k := Identity()
	hidden := 1 - v
	switch edge {
	case DirLeft:
		k.Trim.Right = hidden
	case DirRight:
		k.Trim.Left = hidden
	case DirUp:
		k.Trim.Bottom = hidden
	default:
		k.Trim.Top = hidden
	}
	return k
}

// WipeIn reveals an element over dur seconds from the edge in direction from
// across to the opposite one: DirLeft uncovers it left to right, DirDefault
// bottom to top. It is a clip, so nothing moves.
func WipeIn(t, dur float64, from Direction) Composite {
	return wipeTrim(from, EaseInOutCubic(Progress(t, 0, dur)))
}

// WipeOut hides an element over dur seconds into the edge in direction to, the
// reverse of WipeIn: DirLeft leaves the left edge showing last.
func WipeOut(t, dur float64, to Direction) Composite {
	return wipeTrim(to, 1-EaseInOutCubic(Progress(t, 0, dur)))
}

// Grow swells an element about its center to peak times its size (try 1.1)
// and back over dur seconds, to draw the eye. It is Identity outside 0..dur.
func Grow(t, dur, peak float64) Composite {
	p := Progress(t, 0, dur)
	if p <= 0 || p >= 1 {
		return Identity()
	}
	k := Identity()
	k.Scale = 1 + (peak-1)*math.Sin(math.Pi*p)
	return k
}

// Shake jolts an element by up to amp pixels for dur seconds, dying away,
// 30 times a second. Offsets are whole pixels, so the element stays sharp. It
// is Identity outside 0..dur.
func Shake(t, dur, amp float64) Composite {
	p := Progress(t, 0, dur)
	if p <= 0 || p >= 1 {
		return Identity()
	}
	frame := int(t * 30)
	decay := 1 - p
	k := Identity()
	k.DX = math.Round(amp * decay * (Hash01(frame, 0, 41) - 0.5) * 2)
	k.DY = math.Round(amp * decay * (Hash01(frame, 0, 42) - 0.5) * 2)
	return k
}

// Dim fades an element toward the background, over dur seconds, down to the
// opacity level (try 0.35), and holds it there. To undim it, stop calling Dim
// at the step it should return.
func Dim(t, dur, level float64) Composite {
	return shown(Lerp(1, Clamp01(level), EaseInOutQuad(Progress(t, 0, dur))))
}

// AppearAt turns "built in at step enter, out at step exit" into the
// Composite for this frame. in and out make the entrance and exit from the
// seconds since their step began (the closure sees Ctx), such as
//
//	func(t float64) Composite { return FlyIn(c, t, 0.5, DirLeft, 0.1) }
//
// The element is hidden before enter. A nil in shows it at once, and enter < 0
// has no entrance; exit < 0 never exits, and a nil out cuts it at exit.
// Slides entered backwards or past a step see [Settled] times, so the element
// is already in or already out.
func AppearAt(c Ctx, enter, exit int, in, out func(t float64) Composite) Composite {
	if enter >= 0 && !c.Reached(enter) {
		return Composite{}
	}
	k := Identity()
	if enter >= 0 && in != nil {
		k = in(c.Since(enter))
	}
	if exit >= 0 {
		switch {
		case out == nil && c.Reached(exit):
			return Composite{}
		case out != nil:
			k = k.Then(out(c.Since(exit)))
		}
	}
	return k
}
