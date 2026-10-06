package decker

import "testing"

func TestFullyWipedElementDrawsNothing(t *testing.T) {
	box := NewRect(30, 20, 40, 20)
	// Glows straddling every edge, so anything outside the box is in play.
	strays := func(p *Pixels) {
		p.Rect(box.X, box.Y, box.W, box.H, RGB{255, 255, 255}, 1)
		for _, e := range [][2]float64{{box.X, box.Y + box.H/2}, {box.Right(), box.Y + box.H/2}, {box.X + box.W/2, box.Y}, {box.X + box.W/2, box.Bottom()}} {
			p.Glow(e[0], e[1], 3, RGB{255, 255, 255}, 1)
		}
	}
	for _, dir := range []Direction{DirDefault, DirLeft, DirRight, DirUp, DirDown} {
		for name, k := range map[string]Composite{
			"WipeIn before its start": WipeIn(-1, 0.5, dir),
			"WipeIn at t=0":           WipeIn(0, 0.5, dir),
			"WipeOut settled":         WipeOut(Settled, 0.5, dir),
			"trimmed to nothing":      {Alpha: 1, Trim: Trim{Left: 0.5, Right: 0.5}},
			"vertically to nothing":   {Alpha: 1, Trim: Trim{Top: 0.3, Bottom: 0.7}},
		} {
			p := NewPixels(100, 60, RGB{})
			k.Draw(p, box, strays)
			for i, v := range p.Pix {
				if v != (RGB{}) {
					t.Fatalf("%s, direction %d: pixel (%d,%d) = %v, want nothing drawn", name, dir, i%p.W, i/p.W, v)
				}
			}
		}
	}
}

func TestPartialTrimDoesNotLeakPastTheHiddenSide(t *testing.T) {
	box := NewRect(30, 20, 40, 20)
	glow := func(p *Pixels) { p.Glow(box.X, box.Y+box.H/2, 3, RGB{255, 255, 255}, 1) }
	// 95% hidden from the right leaves a sliver at the left edge; the glow
	// straying left of the box must shrink with it.
	full, sliver := NewPixels(100, 60, RGB{}), NewPixels(100, 60, RGB{})
	Identity().Draw(full, box, glow)
	Composite{Alpha: 1, Trim: Trim{Right: 0.95}}.Draw(sliver, box, glow)
	count := func(p *Pixels) (n int) {
		for x := 0; x < int(box.X); x++ {
			for y := 0; y < p.H; y++ {
				if p.At(x, y) != (RGB{}) {
					n++
				}
			}
		}
		return n
	}
	if f, s := count(full), count(sliver); s >= f/2 {
		t.Errorf("glow left of the box: %d pixels untrimmed, %d with 95%% trimmed away; want far fewer", f, s)
	}
}
