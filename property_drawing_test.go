package decker

import (
	"math"
	"testing"
)

func closeRGB(a, b RGB, tolerance float64) bool {
	return math.Abs(float64(a.R-b.R)) <= tolerance && math.Abs(float64(a.G-b.G)) <= tolerance && math.Abs(float64(a.B-b.B)) <= tolerance
}

func FuzzRasterResampling(f *testing.F) {
	for _, size := range [][4]uint8{{1, 1, 1, 1}, {2, 3, 7, 5}, {8, 7, 1, 1}, {4, 4, 4, 4}} {
		f.Add(size[0], size[1], size[2], size[3], []byte{255, 0, 127, 128, 3, 4, 5, 0})
	}
	f.Fuzz(func(t *testing.T, swb, shb, dwb, dhb uint8, data []byte) {
		sw, sh, dw, dh := int(swb%8)+1, int(shb%8)+1, int(dwb%8)+1, int(dhb%8)+1
		if len(data) == 0 {
			data = []byte{0}
		}
		src := raster{pix: make([]RGB, sw*sh), alpha: make([]float32, sw*sh)}
		for i := range src.pix {
			src.pix[i] = RGB{float32(data[(4*i)%len(data)]), float32(data[(4*i+1)%len(data)]), float32(data[(4*i+2)%len(data)])}
			src.alpha[i] = float32(data[(4*i+3)%len(data)]) / 255
		}
		dst := raster{pix: make([]RGB, dw*dh), alpha: make([]float32, dw*dh)}
		boxScaleAlpha(dst, dw, dh, src, sw, sh)
		for y := range dh {
			for x := range dw {
				// Independently enumerate source pixels in each destination
				// footprint, accumulating in double precision.
				var sum [3]float64
				var alpha, count float64
				for sy := range sh {
					for sx := range sw {
						if sx < x*sw/dw || sx >= max((x+1)*sw/dw, x*sw/dw+1) || sy < y*sh/dh || sy >= max((y+1)*sh/dh, y*sh/dh+1) {
							continue
						}
						i := sy*sw + sx
						a, c := float64(src.alpha[i]), src.pix[i]
						sum[0] += float64(c.R) * a
						sum[1] += float64(c.G) * a
						sum[2] += float64(c.B) * a
						alpha += a
						count++
					}
				}
				want := RGB{}
				if alpha > 0 {
					want = RGB{float32(sum[0] / alpha), float32(sum[1] / alpha), float32(sum[2] / alpha)}
				}
				if i := y*dw + x; !closeRGB(dst.pix[i], want, 0.001) || math.Abs(float64(dst.alpha[i])-alpha/count) > 1e-6 {
					t.Fatalf("resampling %dx%d to %dx%d at (%d,%d): (%v,%g), want (%v,%g)", sw, sh, dw, dh, x, y, dst.pix[i], dst.alpha[i], want, alpha/count)
				}
			}
		}
		// Opaque alpha must reduce to the existing color-only path.
		for i := range src.alpha {
			src.alpha[i] = 1
		}
		boxScaleAlpha(dst, dw, dh, src, sw, sh)
		plain := make([]RGB, dw*dh)
		boxScale(plain, dw, dh, src.pix, sw, sh)
		for i, c := range plain {
			if c != dst.pix[i] || dst.alpha[i] != 1 {
				t.Fatal("opaque resampling differs between alpha and plain paths")
			}
		}
	})
}

func FuzzCompositeTranslation(f *testing.F) {
	for _, alpha := range []uint8{0, 1, 64, 128, 255} {
		f.Add(int8(-3), int8(2), alpha)
	}
	f.Fuzz(func(t *testing.T, xb, yb int8, ab uint8) {
		dx, dy, alpha := float64(xb%12), float64(yb%12), float64(ab)/255
		bg, ink := RGB{12, 34, 56}, RGB{231, 173, 89}
		a, b := NewPixels(16, 16, bg), NewPixels(16, 16, bg)
		r := Rect{4, 4, 6, 6}
		Composite{Alpha: alpha, DX: dx, DY: dy}.Draw(a, r, func(p *Pixels) {
			p.Rect(r.X, r.Y, r.W, r.H, ink, 0.7)
		})
		b.Rect(r.X+dx, r.Y+dy, r.W, r.H, ink, 0.7*alpha)
		for i, c := range a.Pix {
			if !closeRGB(c, b.Pix[i], 0.001) {
				t.Fatalf("whole-pixel move (%g,%g), alpha=%g: pixel %d = %v, want %v", dx, dy, alpha, i, c, b.Pix[i])
			}
		}
	})
}

func TestClippedWideTransition(t *testing.T) {
	for _, tr := range []Transition{TransitionPush, TransitionUncover, TransitionCover} {
		a, b := NewScene(4, 1, testTheme), NewScene(4, 1, testTheme)
		a.Text(3, 0, "界", nil)
		mixTransition(tr.From(DirRight), a, b, 0.5, true, testTheme)
		g := b.toGrid()
		for _, c := range g.Cells {
			if c.ch == "界" {
				t.Fatal("transition resurrected a character clipped out of its source frame")
			}
		}
		g.release()
		a.Release()
		b.Release()
	}
}
