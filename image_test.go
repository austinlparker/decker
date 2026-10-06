package decker

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
	"testing/fstest"
)

// alphaFS holds a 4x4 image whose left half is transparent (with a black,
// zero-alpha color that must not leak), whose top-right is opaque red and whose
// bottom-right is half-transparent blue, plus an opaque 2x1 image.
func alphaFS(t *testing.T) fstest.MapFS {
	t.Helper()
	enc := func(img image.Image) *fstest.MapFile {
		var b bytes.Buffer
		if err := png.Encode(&b, img); err != nil {
			t.Fatal(err)
		}
		return &fstest.MapFile{Data: b.Bytes()}
	}
	a := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 2; x < 4; x++ {
			if y < 2 {
				a.SetNRGBA(x, y, color.NRGBA{255, 0, 0, 255})
			} else {
				a.SetNRGBA(x, y, color.NRGBA{0, 0, 255, 128})
			}
		}
	}
	o := image.NewRGBA(image.Rect(0, 0, 2, 1))
	o.SetRGBA(0, 0, color.RGBA{200, 100, 50, 255})
	o.SetRGBA(1, 0, color.RGBA{10, 20, 30, 255})
	return fstest.MapFS{"img/alpha.png": enc(a), "img/opaque.png": enc(o)}
}

func filled(w, h int, c RGB) *Pixels {
	return NewPixels(w, h, c)
}

// near allows the rounding a 16-bit alpha round trip costs.
func near(a, b RGB) bool {
	d := func(x, y float32) bool { return x-y < 1.01 && y-x < 1.01 }
	return d(a.R, b.R) && d(a.G, b.G) && d(a.B, b.B)
}

func TestImagesTransparentPixelsLeaveCanvas(t *testing.T) {
	bg := RGB{10, 200, 90}
	p := filled(4, 4, bg)
	ims := NewImages(alphaFS(t), "img")
	if _, _, _, _, ok := ims.Draw(p, "alpha.png", 0, 0, 4, 4, 1); !ok {
		t.Fatal("not drawn")
	}
	for y := 0; y < 4; y++ {
		for x := 0; x < 2; x++ {
			if p.Pix[y*4+x] != bg {
				t.Fatalf("transparent pixel (%d,%d) changed to %v", x, y, p.Pix[y*4+x])
			}
		}
	}
	if got := p.Pix[0*4+3]; got != (RGB{255, 0, 0}) {
		t.Errorf("opaque pixel = %v, want red", got)
	}
	// Half-alpha blue over bg: 128/255 blue, the rest bg (blue not darkened).
	k := float32(128) / 255
	want := RGB{bg.R * (1 - k), bg.G * (1 - k), bg.B*(1-k) + 255*k}
	if got := p.Pix[3*4+3]; !near(got, want) {
		t.Errorf("half-alpha pixel = %v, want about %v", got, want)
	}
}

func TestImagesAlphaMultipliesCallerAlpha(t *testing.T) {
	p := filled(4, 4, RGB{})
	NewImages(alphaFS(t), "img").Draw(p, "alpha.png", 0, 0, 4, 4, 0.5)
	if got := p.Pix[0*4+3]; !near(got, RGB{127.5, 0, 0}) {
		t.Errorf("opaque pixel at alpha 0.5 = %v", got)
	}
	k := float32(0.5 * 128 / 255)
	if got := p.Pix[3*4+3]; !near(got, RGB{0, 0, 255 * k}) {
		t.Errorf("half-alpha pixel at alpha 0.5 = %v", got)
	}
}

func TestImagesScaledAlphaKeepsEdgeColor(t *testing.T) {
	// Shrunk 4x4 to 2x2: the left cells are fully transparent, the top-right is
	// opaque red and the bottom-right half-transparent blue.
	white := RGB{255, 255, 255}
	p := filled(2, 2, white)
	NewImages(alphaFS(t), "img").Draw(p, "alpha.png", 0, 0, 2, 2, 1)
	if p.Pix[0] != white || p.Pix[2] != white {
		t.Errorf("transparent cells changed: %v %v", p.Pix[0], p.Pix[2])
	}
	if p.Pix[1] != (RGB{255, 0, 0}) {
		t.Errorf("opaque cell = %v", p.Pix[1])
	}
	k := float32(128) / 255
	if want := (RGB{255 * (1 - k), 255 * (1 - k), 255}); !near(p.Pix[3], want) {
		t.Errorf("blue cell = %v, want about %v", p.Pix[3], want)
	}

	// Shrunk to 1x1: a quarter red, a quarter blue at half alpha, half clear.
	// Weighting by alpha keeps the red from being averaged with black.
	q := filled(1, 1, RGB{})
	NewImages(alphaFS(t), "img").Draw(q, "alpha.png", 0, 0, 1, 1, 1)
	a := (1 + float32(128)/255) / 4
	want := RGB{255 * (1 / (1 + float32(128)/255)) * a, 0, 255 * (float32(128) / 255 / (1 + float32(128)/255)) * a}
	if !near(q.Pix[0], want) {
		t.Errorf("1x1 = %v, want about %v", q.Pix[0], want)
	}
}

func TestImagesOpaqueHasNoAlphaPlane(t *testing.T) {
	ims := NewImages(alphaFS(t), "img")
	if ims.load("opaque.png").alpha != nil {
		t.Error("opaque image kept an alpha plane")
	}
	if ims.load("alpha.png").alpha == nil {
		t.Error("transparent image lost its alpha plane")
	}
	p := filled(2, 1, RGB{})
	ims.Draw(p, "opaque.png", 0, 0, 2, 1, 1)
	if p.Pix[0] != (RGB{200, 100, 50}) || p.Pix[1] != (RGB{10, 20, 30}) {
		t.Errorf("opaque draw = %v", p.Pix)
	}
}

func TestImagesDrawCoverFillsBox(t *testing.T) {
	ims := NewImages(alphaFS(t), "img")
	// 2x1 image into a 6x6 box: cover scales to 12x6 and crops 3 from each side,
	// leaving three columns of each source pixel.
	bg := RGB{1, 2, 3}
	p := filled(8, 8, bg)
	x, y, w, h, ok := ims.DrawCover(p, "opaque.png", 1, 1, 6, 6, 1)
	if !ok || x != 1 || y != 1 || w != 6 || h != 6 {
		t.Fatalf("DrawCover = %v %v %v %v %v", x, y, w, h, ok)
	}
	for py := 0; py < 8; py++ {
		for px := 0; px < 8; px++ {
			in := px >= 1 && px < 7 && py >= 1 && py < 7
			got := p.Pix[py*8+px]
			switch {
			case !in && got != bg:
				t.Fatalf("pixel (%d,%d) outside the box changed", px, py)
			case in && got == bg:
				t.Fatalf("pixel (%d,%d) inside the box not filled", px, py)
			}
		}
	}
	if p.Pix[1*8+1] != (RGB{200, 100, 50}) || p.Pix[1*8+6] != (RGB{10, 20, 30}) {
		t.Errorf("cover crop not centered: %v %v", p.Pix[1*8+1], p.Pix[1*8+6])
	}

	// Draw letterboxes the same image, so the corner of the box stays clear.
	q := filled(8, 8, bg)
	ims.Draw(q, "opaque.png", 1, 1, 6, 6, 1)
	if q.Pix[1*8+1] != bg {
		t.Error("Draw filled the box; the test would not tell Draw and DrawCover apart")
	}
}

func TestImagesDrawCoverTransparentAndMissing(t *testing.T) {
	ims := NewImages(alphaFS(t), "img")
	bg := RGB{5, 5, 5}
	p := filled(4, 2, bg)
	// 4x4 image into a 4x2 box: the middle rows show; the left half stays clear.
	if _, _, _, _, ok := ims.DrawCover(p, "alpha.png", 0, 0, 4, 2, 1); !ok {
		t.Fatal("not drawn")
	}
	if p.Pix[0] != bg || p.Pix[4] != bg {
		t.Error("transparent left half changed")
	}
	if p.Pix[3] == bg || p.Pix[7] == bg {
		t.Errorf("right column not drawn: %v %v", p.Pix[3], p.Pix[7])
	}
	if _, _, _, _, ok := ims.DrawCover(p, "nope.png", 0, 0, 4, 2, 1); ok {
		t.Error("missing image drawn")
	}
	if _, _, _, _, ok := ims.DrawCover(p, "alpha.png", 0, 0, 0.5, 2, 1); ok {
		t.Error("sub-pixel box drawn")
	}
}
