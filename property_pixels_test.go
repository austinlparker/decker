package decker

import (
	"math"
	"math/rand/v2"
	"testing"
)

// The byte programs cover empty canvases, clipping, opacity endpoints, and
// arbitrary sequences of mutations against a separate flat framebuffer.
func FuzzPixels(f *testing.F) {
	f.Add(uint8(0), uint8(0), []byte{})
	f.Add(uint8(4), uint8(3), []byte{0, 1, 1, 255, 0, 0, 255})
	rng := rand.New(rand.NewPCG(1, 2))
	for range 64 {
		program := make([]byte, 7*32)
		for i := range program {
			program[i] = byte(rng.Uint32())
		}
		f.Add(uint8(rng.IntN(9)), uint8(rng.IntN(9)), program)
	}
	f.Fuzz(func(t *testing.T, wb, hb uint8, program []byte) {
		w, h := int(wb%9), int(hb%9)
		p := NewPixels(w, h, RGB{12, 34, 56})
		want := make([]RGB, w*h)
		for i := range want {
			want[i] = RGB{12, 34, 56}
		}
		for n := 0; n+7 <= len(program) && n < 7*256; n += 7 {
			b := program[n : n+7]
			x, y := int(int8(b[1])), int(int8(b[2]))
			c := RGB{float32(b[3]), float32(b[4]), float32(b[5])}
			a := float64(b[6])/128 - 0.5
			inside := x >= 0 && y >= 0 && x < w && y < h
			switch b[0] % 5 {
			case 0:
				p.Set(x, y, c)
				if inside {
					want[y*w+x] = c
				}
			case 1:
				p.Blend(x, y, c, a)
				if inside && a > 0 {
					d := want[y*w+x]
					q := float32(min(a, 1))
					want[y*w+x] = RGB{d.R + (c.R-d.R)*q, d.G + (c.G-d.G)*q, d.B + (c.B-d.B)*q}
				}
			case 2:
				p.Add(x, y, c, a)
				if inside && a > 0 {
					d, q := want[y*w+x], float32(a)
					want[y*w+x] = RGB{min(d.R+c.R*q, 255), min(d.G+c.G*q, 255), min(d.B+c.B*q, 255)}
				}
			case 3:
				p.Fill(c)
				for i := range want {
					want[i] = c
				}
			case 4:
				expected := RGB{}
				if inside {
					expected = want[y*w+x]
				}
				if got := p.At(x, y); got != expected {
					t.Fatalf("At(%d,%d) = %v, want %v", x, y, got, expected)
				}
			}
			for i, c := range p.Pix {
				if c != want[i] {
					t.Fatalf("operation %d: pixel %d = %v, want %v", n/7, i, c, want[i])
				}
			}
			if p.BG != (RGB{12, 34, 56}) {
				t.Fatal("drawing changed BG")
			}
		}
	})
}

func FuzzPixelBox(f *testing.F) {
	for _, b := range [][4]float64{
		{0, 0, 3, 3}, {-0.5, -0.5, 1.5, 1.5}, {5, 5, 2, 2},
		{-math.MaxFloat64, -math.MaxFloat64, math.MaxFloat64, math.MaxFloat64},
		{math.MaxFloat64, 0, math.MaxFloat64, 1}, {0, 0, -math.MaxFloat64, 1},
	} {
		f.Add(b[0], b[1], b[2], b[3])
	}
	f.Fuzz(func(t *testing.T, x0, y0, x1, y1 float64) {
		if !finite(x0) || !finite(y0) || !finite(x1) || !finite(y1) {
			return // Non-finite geometry has no defined coverage.
		}
		p := NewPixels(7, 5, RGB{})
		a, b, c, d := p.Box(x0, y0, x1, y1)
		// Compare membership, not a second float-to-int conversion: overflow
		// outside the canvas must never turn a covering box into an empty one.
		for y := range p.H {
			for x := range p.W {
				want := float64(x) >= math.Floor(x0) && float64(x) <= math.Ceil(x1) &&
					float64(y) >= math.Floor(y0) && float64(y) <= math.Ceil(y1)
				got := x >= a && x <= c && y >= b && y <= d
				if got != want {
					t.Fatalf("Box(%g,%g,%g,%g) = (%d,%d,%d,%d): membership at (%d,%d) = %v, want %v", x0, y0, x1, y1, a, b, c, d, x, y, got, want)
				}
			}
		}
		if a <= c && b <= d && (a < 0 || b < 0 || c >= p.W || d >= p.H) {
			t.Fatal("nonempty box escaped canvas")
		}
	})
}
