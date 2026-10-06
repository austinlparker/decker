package decker

import (
	"math"
	"testing"
)

func TestEasingEndpoints(t *testing.T) {
	cb := CubicBezier(0.25, 0.1, 0.25, 1)
	for name, f := range map[string]func(float64) float64{
		"EaseInQuad": EaseInQuad, "EaseOutQuad": EaseOutQuad, "EaseInOutQuad": EaseInOutQuad,
		"EaseInCubic": EaseInCubic, "EaseOutCubic": EaseOutCubic, "EaseInOutCubic": EaseInOutCubic,
		"EaseOutExpo": EaseOutExpo, "EaseInOutExpo": EaseInOutExpo,
		"EaseOutElastic": EaseOutElastic, "EaseOutBounce": EaseOutBounce, "EaseOutBack": EaseOutBack,
		"CubicBezier": cb,
	} {
		if got := f(0); math.Abs(got) > 1e-9 {
			t.Errorf("%s(0) = %v, want 0", name, got)
		}
		if got := f(1); math.Abs(got-1) > 1e-9 {
			t.Errorf("%s(1) = %v, want 1", name, got)
		}
		if f(-3) != f(0) || f(7) != f(1) {
			t.Errorf("%s does not clamp its input", name)
		}
	}
}

func TestEasingKnownValues(t *testing.T) {
	for _, c := range []struct {
		name string
		got  float64
		want float64
	}{
		{"InQuad(.5)", EaseInQuad(0.5), 0.25},
		{"OutQuad(.5)", EaseOutQuad(0.5), 0.75},
		{"InOutQuad(.25)", EaseInOutQuad(0.25), 0.125},
		{"InOutQuad(.5)", EaseInOutQuad(0.5), 0.5},
		{"InOutQuad(.75)", EaseInOutQuad(0.75), 0.875},
		{"InCubic(.5)", EaseInCubic(0.5), 0.125},
		{"OutExpo(.5)", EaseOutExpo(0.5), 1 - 1.0/32},
		{"InOutExpo(.5)", EaseInOutExpo(0.5), 0.5},
		{"OutBounce(.5)", EaseOutBounce(0.5), 0.765625},
		{"OutBounce(1/2.75)", EaseOutBounce(1 / 2.75), 1},
		{"OutElastic(.5)", EaseOutElastic(0.5), 1.015625},
	} {
		if math.Abs(c.got-c.want) > 1e-9 {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
	if EaseOutElastic(0.1) <= 1 {
		t.Error("EaseOutElastic should overshoot 1 early on")
	}
}

func TestEasingsAreMonotonicWhereTheyShouldBe(t *testing.T) {
	for name, f := range map[string]func(float64) float64{
		"InQuad": EaseInQuad, "OutQuad": EaseOutQuad, "InOutQuad": EaseInOutQuad, "InCubic": EaseInCubic,
		"OutExpo": EaseOutExpo, "InOutExpo": EaseInOutExpo, "CubicBezier": CubicBezier(0.42, 0, 0.58, 1),
	} {
		prev := -1.0
		for i := 0; i <= 200; i++ {
			v := f(float64(i) / 200)
			if v < prev-1e-9 {
				t.Fatalf("%s decreases at %d/200: %v < %v", name, i, v, prev)
			}
			prev = v
		}
	}
}

func TestCubicBezierMatchesCSS(t *testing.T) {
	ease := CubicBezier(0.25, 0.1, 0.25, 1)
	// Values of CSS "ease", from the curve itself: solve x(t)=p by sweeping t.
	for _, p := range []float64{0.05, 0.1, 0.25, 0.5, 0.75, 0.9, 0.99} {
		best, by := 2.0, 0.0
		for i := 0; i <= 200000; i++ {
			s := float64(i) / 200000
			x := 3*(1-s)*(1-s)*s*0.25 + 3*(1-s)*s*s*0.25 + s*s*s
			if d := math.Abs(x - p); d < best {
				best = d
				by = 3*(1-s)*(1-s)*s*0.1 + 3*(1-s)*s*s*1 + s*s*s
			}
		}
		if got := ease(p); math.Abs(got-by) > 1e-4 {
			t.Errorf("ease(%v) = %v, want %v", p, got, by)
		}
	}
	// The value browsers show half-way through "ease".
	if got := ease(0.5); math.Abs(got-0.8024) > 5e-4 {
		t.Errorf("ease(0.5) = %v, want ~0.8024", got)
	}
	// A straight line through the corners is linear.
	line := CubicBezier(0, 0, 1, 1)
	for _, p := range []float64{0.1, 0.5, 0.9} {
		if got := line(p); math.Abs(got-p) > 1e-6 {
			t.Errorf("linear bezier(%v) = %v", p, got)
		}
	}
	// Steep curves (control x at the extremes) still solve.
	steep := CubicBezier(1, 0, 0, 1)
	if got := steep(0.5); math.Abs(got-0.5) > 1e-6 {
		t.Errorf("steep bezier(0.5) = %v, want 0.5 by symmetry", got)
	}
}

func TestCubicBezierDoesNotAllocate(t *testing.T) {
	f := CubicBezier(0.25, 0.1, 0.25, 1)
	if n := testing.AllocsPerRun(100, func() { f(0.37) }); n != 0 {
		t.Errorf("a CubicBezier call allocates %v times", n)
	}
}

func TestEntrancesStartHiddenAndSettle(t *testing.T) {
	c := Ctx{W: 100, H: 30}
	in := map[string]func(t float64) Composite{
		"FadeIn": func(t float64) Composite { return FadeIn(t, 0.5) },
		"FlyIn":  func(t float64) Composite { return FlyIn(c, t, 0.5, DirLeft, 0.2) },
		"ZoomIn": func(t float64) Composite { return ZoomIn(t, 0.5, 0.6) },
		"Pop":    func(t float64) Composite { return Pop(t, 0.5) },
		"WipeIn": func(t float64) Composite { return WipeIn(t, 0.5, DirUp) },
	}
	for name, f := range in {
		if k := f(0); k.Alpha > 0 && k.Trim == (Trim{}) && k.scale() >= 0.01 {
			t.Errorf("%s at t=0 = %+v, want hidden", name, k)
		}
		if k := f(Settled); k != Identity() {
			t.Errorf("%s settled = %+v, want Identity", name, k)
		}
		if k := f(-1); k != f(0) {
			t.Errorf("%s before its start = %+v, want the start state %+v", name, k, f(0))
		}
	}
}

func TestExitsStartVisibleAndEndHidden(t *testing.T) {
	c := Ctx{W: 100, H: 30}
	out := map[string]func(t float64) Composite{
		"FadeOut": func(t float64) Composite { return FadeOut(t, 0.5) },
		"FlyOut":  func(t float64) Composite { return FlyOut(c, t, 0.5, DirRight, 0.2) },
		"ZoomOut": func(t float64) Composite { return ZoomOut(t, 0.5, 0.6) },
		"WipeOut": func(t float64) Composite { return WipeOut(t, 0.5, DirDown) },
	}
	for name, f := range out {
		if k := f(0); k != Identity() {
			t.Errorf("%s at t=0 = %+v, want Identity", name, k)
		}
		if k := f(-1); k != Identity() {
			t.Errorf("%s before its start = %+v, want Identity", name, k)
		}
		k := f(Settled)
		if k.Alpha > 0 && k.Trim.Left < 1 && k.Trim.Right < 1 && k.Trim.Top < 1 && k.Trim.Bottom < 1 {
			t.Errorf("%s settled = %+v, want hidden", name, k)
		}
	}
}

func TestEmphasisReturnsToIdentity(t *testing.T) {
	for name, k := range map[string]Composite{
		"Grow before":  Grow(-1, 0.5, 1.2),
		"Grow after":   Grow(0.5, 0.5, 1.2),
		"Shake before": Shake(-1, 0.5, 6),
		"Shake after":  Shake(0.6, 0.5, 6),
	} {
		if k != Identity() {
			t.Errorf("%s = %+v, want Identity", name, k)
		}
	}
	if s := Grow(0.25, 0.5, 1.2).Scale; math.Abs(s-1.2) > 1e-9 {
		t.Errorf("Grow at its middle = %v, want the peak 1.2", s)
	}
	if k := Dim(Settled, 0.3, 0.35); math.Abs(k.Alpha-0.35) > 1e-9 {
		t.Errorf("Dim settled = %+v, want alpha 0.35", k)
	}
	if k := Dim(-1, 0.3, 0.35); k != Identity() {
		t.Errorf("Dim before = %+v, want Identity", k)
	}
	a, b := Shake(0.2, 1, 6), Shake(0.2, 1, 6)
	if a != b || a.DX != math.Round(a.DX) || a.DY != math.Round(a.DY) {
		t.Errorf("Shake = %+v then %+v, want pure and whole pixels", a, b)
	}
}

func TestFlyInDirections(t *testing.T) {
	c := Ctx{W: 100, H: 30} // 100x60 pixels
	for _, tc := range []struct {
		from   Direction
		dx, dy float64
	}{{DirLeft, -20, 0}, {DirRight, 20, 0}, {DirUp, 0, -12}, {DirDown, 0, 12}} {
		k := FlyIn(c, 0, 1, tc.from, 0.2)
		if math.Abs(k.DX-tc.dx) > 1e-9 || math.Abs(k.DY-tc.dy) > 1e-9 {
			t.Errorf("FlyIn from %d starts at (%v, %v), want (%v, %v)", tc.from, k.DX, k.DY, tc.dx, tc.dy)
		}
	}
}

func TestWipeInRevealsFromTheSide(t *testing.T) {
	box := NewRect(10, 10, 80, 20)
	fill := func(p *Pixels) { p.Rect(box.X, box.Y, box.W, box.H, RGB{255, 255, 255}, 1) }
	p := NewPixels(100, 60, RGB{})
	WipeIn(0.25, 0.5, DirLeft).Draw(p, box, fill) // half way: EaseInOutCubic(0.5) = 0.5
	if p.At(20, 20).R != 255 || p.At(80, 20).R != 0 {
		t.Errorf("WipeIn from the left: x=20 is %v, x=80 is %v", p.At(20, 20), p.At(80, 20))
	}
	q := NewPixels(100, 60, RGB{})
	WipeOut(0.25, 0.5, DirLeft).Draw(q, box, fill)
	if q.At(20, 20).R != 255 || q.At(80, 20).R != 0 {
		t.Errorf("WipeOut to the left: x=20 is %v, x=80 is %v", q.At(20, 20), q.At(80, 20))
	}
}

func TestAppearAt(t *testing.T) {
	in := func(t float64) Composite { return FadeIn(t, 1) }
	out := func(t float64) Composite { return FadeOut(t, 1) }
	at := func(step int, stepT float64) Composite {
		return AppearAt(Ctx{Step: step, StepT: stepT}, 1, 3, in, out)
	}
	if k := at(0, 5); k.Alpha != 0 {
		t.Errorf("before enter: %+v, want hidden", k)
	}
	if k := at(1, 0.5); k.Alpha <= 0 || k.Alpha >= 1 {
		t.Errorf("entering: %+v, want partly faded", k)
	}
	if k := at(2, 0.1); k != Identity() {
		t.Errorf("between: %+v, want Identity", k)
	}
	if k := at(3, 0.5); k.Alpha <= 0 || k.Alpha >= 1 {
		t.Errorf("exiting: %+v, want partly faded", k)
	}
	if k := at(4, 0); k.Alpha != 0 {
		t.Errorf("after exit: %+v, want hidden", k)
	}
	// No exit, no entrance function, and a cut exit.
	c := Ctx{Step: 2}
	if k := AppearAt(c, 1, -1, in, out); k != Identity() {
		t.Errorf("never exits: %+v", k)
	}
	if k := AppearAt(c, 3, -1, nil, nil); k.Alpha != 0 {
		t.Errorf("not yet entered, nil in: %+v", k)
	}
	if k := AppearAt(Ctx{Step: 3}, 1, 3, nil, nil); k.Alpha != 0 {
		t.Errorf("cut at exit: %+v", k)
	}
	if k := AppearAt(Ctx{Step: 0}, -1, -1, nil, nil); k != Identity() {
		t.Errorf("always on: %+v", k)
	}
}
