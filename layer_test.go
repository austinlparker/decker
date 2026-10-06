package decker

import (
	"math"
	"testing"
)

// layerCanvas is a striped canvas, so that moving and clipping show.
func layerCanvas() *Pixels {
	p := NewPixels(120, 60, Hex("#102030"))
	for x := 0; x < p.W; x += 8 {
		p.Rect(float64(x), 0, 3, float64(p.H), Hex("#304050"), 1)
	}
	return p
}

var layerBox = NewRect(30, 15, 40, 20)

var layerOrange, layerBlue, layerGreen = Hex("#FF7A00"), Hex("#40C0FF"), Hex("#70D050")

// layerDraw paints an opaque block over layerBox, with an antialiased disc
// and a glow that stays within the margin Draw allows.
func layerDraw(p *Pixels) { layerDrawAt(p, 0, 0, true) }

func layerDrawAt(p *Pixels, dx, dy float64, glow bool) {
	p.RoundRect(layerBox.X+dx, layerBox.Y+dy, layerBox.W, layerBox.H, 4, 0, layerOrange, 1)
	p.Disc(50+dx, 25+dy, 6.5, layerBlue, 0.8)
	if glow {
		p.Glow(70+dx, 25+dy, 5, layerGreen, 0.5)
	}
}

func samePixels(t *testing.T, got, want *Pixels, tol float32) {
	t.Helper()
	for i := range want.Pix {
		g, w := got.Pix[i], want.Pix[i]
		if abs32(g.R-w.R) > tol || abs32(g.G-w.G) > tol || abs32(g.B-w.B) > tol {
			t.Fatalf("pixel (%d,%d) = %v, want %v", i%want.W, i/want.W, g, w)
		}
	}
}

func abs32(v float32) float32 { return float32(math.Abs(float64(v))) }

func TestCompositeIdentityIsADirectDraw(t *testing.T) {
	got, want := layerCanvas(), layerCanvas()
	Identity().Draw(got, layerBox, layerDraw)
	layerDraw(want)
	samePixels(t, got, want, 0)
}

func TestCompositeAlphaZeroDrawsNothing(t *testing.T) {
	got, want := layerCanvas(), layerCanvas()
	called := false
	k := Identity()
	k.Alpha = 0
	k.Draw(got, layerBox, func(*Pixels) { called = true })
	(Composite{}).Draw(got, layerBox, func(*Pixels) { called = true })
	if called {
		t.Error("an invisible element was still drawn")
	}
	samePixels(t, got, want, 0)
}

func TestCompositeAlphaMixesWholeElement(t *testing.T) {
	got, drawn := layerCanvas(), layerCanvas()
	bg := layerCanvas()
	k := Identity()
	k.Alpha = 0.4
	k.Draw(got, layerBox, layerDraw)
	layerDraw(drawn)
	for i := range got.Pix {
		want := Mix(bg.Pix[i], drawn.Pix[i], 0.4)
		if abs32(got.Pix[i].R-want.R) > 1e-3 || abs32(got.Pix[i].G-want.G) > 1e-3 || abs32(got.Pix[i].B-want.B) > 1e-3 {
			t.Fatalf("pixel %d = %v, want %v", i, got.Pix[i], want)
		}
	}
}

func TestCompositeTrimShowsOnlyTheClip(t *testing.T) {
	got, full, bg := layerCanvas(), layerCanvas(), layerCanvas()
	layerDraw(full)
	clip := NewRect(40, 20, 20, 10)
	Identity().Clipped(layerBox, clip).Draw(got, layerBox, layerDraw)
	for y := 0; y < got.H; y++ {
		for x := 0; x < got.W; x++ {
			want := bg.At(x, y)
			if float64(x) >= clip.X && float64(x) < clip.Right() && float64(y) >= clip.Y && float64(y) < clip.Bottom() {
				want = full.At(x, y)
			}
			if g := got.At(x, y); g != want {
				t.Fatalf("pixel (%d,%d) = %v, want %v", x, y, g, want)
			}
		}
	}
}

func TestCompositeMovesWholePixelsExactly(t *testing.T) {
	got, want := layerCanvas(), layerCanvas()
	k := Identity()
	k.DX, k.DY = 17, 9
	k.Draw(got, layerBox, func(p *Pixels) { layerDrawAt(p, 0, 0, false) })
	// The same drawing, shifted: the canvas pattern stays put, the element moves.
	layerDrawAt(want, 17, 9, false)
	samePixels(t, got, want, 0.05)
}

func TestCompositeLayeredEqualsDirectOnOpaque(t *testing.T) {
	// A translation by zero through the layered path (a scale so close to 1
	// it still takes it) must reproduce the direct draw to within rounding.
	// Glow is additive light, which a layer can only screen in, so leave it out.
	got, want := layerCanvas(), layerCanvas()
	k := Identity()
	k.Scale = 1 + 1e-12
	k.DX = 1e-9
	k.Draw(got, layerBox, func(p *Pixels) { layerDrawAt(p, 0, 0, false) })
	layerDrawAt(want, 0, 0, false)
	samePixels(t, got, want, 0.05)
}

func TestCompositeScaleAboutCenter(t *testing.T) {
	p := NewPixels(120, 60, RGB{})
	box := NewRect(40, 20, 40, 20)
	k := Identity()
	k.Scale = 0.5
	k.Draw(p, box, func(p *Pixels) { p.Rect(box.X, box.Y, box.W, box.H, RGB{200, 100, 50}, 1) })
	if g := p.At(60, 30); g != (RGB{200, 100, 50}) {
		t.Errorf("center = %v, want the element's color", g)
	}
	if g := p.At(45, 30); g != (RGB{}) {
		t.Errorf("a pixel outside the half-size element = %v, want background", g)
	}
	if g := p.At(52, 25); g.R < 190 {
		t.Errorf("a pixel inside the half-size element = %v, want solid", g)
	}
	// Pivot at the top-left corner pins that corner.
	q := NewPixels(120, 60, RGB{})
	k.PivotX, k.PivotY = -0.5, -0.5
	k.Draw(q, box, func(p *Pixels) { p.Rect(box.X, box.Y, box.W, box.H, RGB{200, 100, 50}, 1) })
	if g := q.At(41, 21); g.R < 190 {
		t.Errorf("near the pinned corner = %v, want solid", g)
	}
	if g := q.At(70, 35); g != (RGB{}) {
		t.Errorf("far corner of the unscaled box = %v, want background", g)
	}
}

func TestCompositeScaleCoverageIsExactOnOpaque(t *testing.T) {
	// Growing a small solid square into a bigger one: no pixel may come out
	// darker than the color or leak the black or white of the layers.
	p := NewPixels(120, 60, Hex("#102030"))
	box := NewRect(50, 25, 10, 10)
	col := Hex("#FF7A00")
	k := Identity()
	k.Scale = 2.37
	k.Draw(p, box, func(p *Pixels) { p.Rect(box.X, box.Y, box.W, box.H, col, 1) })
	if g := p.At(55, 30); abs32(g.R-col.R) > 0.01 || abs32(g.G-col.G) > 0.01 || abs32(g.B-col.B) > 0.01 {
		t.Errorf("middle of the grown square = %v, want %v", g, col)
	}
	for _, pt := range [][2]int{{0, 0}, {119, 59}, {20, 30}} {
		if g := p.At(pt[0], pt[1]); g != Hex("#102030") {
			t.Errorf("canvas at %v = %v, want it untouched", pt, g)
		}
	}
}

func TestCompositeOffCanvasDoesNotPanic(t *testing.T) {
	p := layerCanvas()
	for _, box := range []Rect{NewRect(-50, -50, 20, 20), NewRect(100, 40, 80, 80), NewRect(10, 10, 0, 0)} {
		k := Identity()
		k.DX, k.Scale, k.Alpha = 500, 0.3, 0.5
		k.Draw(p, box, layerDraw)
		k.DX = -500
		k.Draw(p, box, layerDraw)
		Identity().Clipped(box, NewRect(0, 0, 5, 5)).Draw(p, box, layerDraw)
	}
}

func TestCompositeThen(t *testing.T) {
	a := Composite{Alpha: 0.5, DX: 1, Scale: 2, Trim: Trim{Left: 0.2}}
	b := Composite{Alpha: 0.5, DY: 3, Trim: Trim{Left: 0.1, Top: 0.3}, PivotX: 0.25}
	got := a.Then(b)
	want := Composite{Alpha: 0.25, DX: 1, DY: 3, Scale: 2, PivotX: 0.25, Trim: Trim{Left: 0.2, Top: 0.3}}
	if got != want {
		t.Errorf("Then = %+v, want %+v", got, want)
	}
	if Combine() != Identity() {
		t.Errorf("Combine() = %+v, want Identity", Combine())
	}
}

func TestCompositeSteadyStateDoesNotAllocate(t *testing.T) {
	// Other tests' differently sized layers may be parked in the pool.
	layers.mu.Lock()
	layers.free = nil
	layers.mu.Unlock()
	p := layerCanvas()
	k := Identity()
	k.DX, k.Scale, k.Alpha = 3.5, 0.8, 0.7
	f := Identity()
	f.Alpha = 0.5
	draw := layerDraw
	k.Draw(p, layerBox, draw) // warms the pool
	f.Draw(p, layerBox, draw)
	if n := testing.AllocsPerRun(20, func() {
		k.Draw(p, layerBox, draw)
		f.Draw(p, layerBox, draw)
	}); n > 0 {
		t.Errorf("Composite.Draw allocates %v times per frame", n)
	}
}

// BenchmarkComposite is a Panel-sized element drawn through each kind of
// Composite onto a 682x171-cell canvas (682x342 pixels).
func BenchmarkComposite(b *testing.B) {
	c := Ctx{W: 682, H: 171, Theme: testTheme}
	p := NewPixels(682, 342, testTheme.Background)
	r := c.Rect(0.3, 0.3, 0.3, 0.3)
	draw := func(p *Pixels) {
		Panel(c, p, r.X, r.Y, r.W, r.H, "panel", testTheme.Panel, testTheme.Accent2, testTheme.Text, 1)
	}
	cases := []struct {
		name string
		k    Composite
	}{
		{"draw", Composite{}},
		{"identity", Identity()},
		{"fade", Composite{Alpha: 0.5, Scale: 1}},
		{"wipe", WipeIn(0.2, 0.5, DirLeft)},
		{"fly", Composite{Alpha: 1, Scale: 1, DX: 40.5, DY: 6.25}},
		{"zoom", ZoomIn(0.2, 0.5, 0.6)},
		{"shrink", Composite{Alpha: 1, Scale: 0.4}},
	}
	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				if tc.name == "draw" {
					draw(p)
					continue
				}
				tc.k.Draw(p, r, draw)
			}
		})
	}
}
