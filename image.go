package decker

import (
	"bytes"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io/fs"
	"math"
	"path"
	"sync"
)

// Images is a set of raster images (PNG or JPEG), usually embedded in the
// binary. Each is decoded on first use, and each drawn size is cached.
type Images struct {
	fsys fs.FS
	dir  string

	mu    sync.Mutex
	cache map[string]*decodedImage
}

// NewImages returns the images in directory dir of fsys.
func NewImages(fsys fs.FS, dir string) *Images {
	return &Images{fsys: fsys, dir: dir, cache: map[string]*decodedImage{}}
}

// raster is color with an optional coverage plane. alpha is nil for an opaque
// image, which keeps the common case on the plain boxScale path.
type raster struct {
	pix   []RGB     // straight (not premultiplied) color
	alpha []float32 // 0..1 per pixel; nil when every pixel is opaque
}

type decodedImage struct {
	w, h int
	raster
	// pix is nil when the file is missing or unreadable.

	mu    sync.Mutex
	cache map[[2]int]raster
}

func (ims *Images) load(name string) *decodedImage {
	ims.mu.Lock()
	defer ims.mu.Unlock()
	if im, ok := ims.cache[name]; ok {
		return im
	}
	im := &decodedImage{cache: map[[2]int]raster{}}
	if data, err := fs.ReadFile(ims.fsys, path.Join(ims.dir, name)); err == nil {
		if src, _, err := image.Decode(bytes.NewReader(data)); err == nil {
			b := src.Bounds()
			im.w, im.h = b.Dx(), b.Dy()
			im.pix = make([]RGB, im.w*im.h)
			var alpha []float32
			for y := 0; y < im.h; y++ {
				for x := 0; x < im.w; x++ {
					c, a := toRGBA(src.At(b.Min.X+x, b.Min.Y+y))
					i := y*im.w + x
					im.pix[i] = c
					if a < 1 && alpha == nil {
						alpha = make([]float32, im.w*im.h)
						for j := 0; j < i; j++ {
							alpha[j] = 1
						}
					}
					if alpha != nil {
						alpha[i] = a
					}
				}
			}
			im.alpha = alpha
		}
	}
	ims.cache[name] = im
	return im
}

// Has reports whether the image called name exists and decodes.
func (ims *Images) Has(name string) bool { return ims.load(name).pix != nil }

// Draw paints the image called name into the box (x, y, w, h), aspect kept,
// centered, at opacity alpha. Source pixels are averaged per canvas pixel and
// the resampled copy is cached per size. A transparent image lets the canvas
// show through in proportion to each pixel's own alpha. It returns the drawn
// rectangle, or ok=false (drawing nothing) if the image is missing.
func (ims *Images) Draw(p *Pixels, name string, x, y, w, h, alpha float64) (dx, dy, dw, dh float64, ok bool) {
	im := ims.load(name)
	if im.pix == nil || w < 1 || h < 1 {
		return 0, 0, 0, 0, false
	}
	s := min(w/float64(im.w), h/float64(im.h))
	sw, sh := max(int(float64(im.w)*s), 1), max(int(float64(im.h)*s), 1)
	scaled := im.scaled(sw, sh)
	ix, iy := int(math.Round(x+(w-float64(sw))/2)), int(math.Round(y+(h-float64(sh))/2))
	scaled.blit(p, ix, iy, sw, 0, 0, sw, sh, alpha)
	return float64(ix), float64(iy), float64(sw), float64(sh), true
}

// DrawCover paints the image called name so it fills the box (x, y, w, h)
// completely: the image is scaled until both sides cover the box, keeping its
// aspect, and the overflow is cropped evenly from both ends. Opacity, caching
// and transparency work as in Draw. It returns the filled rectangle, which is
// the box rounded to whole pixels, or ok=false (drawing nothing) if the image
// is missing.
func (ims *Images) DrawCover(p *Pixels, name string, x, y, w, h, alpha float64) (dx, dy, dw, dh float64, ok bool) {
	im := ims.load(name)
	if im.pix == nil || w < 1 || h < 1 {
		return 0, 0, 0, 0, false
	}
	x0, y0 := int(math.Round(x)), int(math.Round(y))
	bw, bh := max(int(math.Round(x+w))-x0, 1), max(int(math.Round(y+h))-y0, 1)
	s := max(float64(bw)/float64(im.w), float64(bh)/float64(im.h))
	sw, sh := max(int(math.Ceil(float64(im.w)*s)), bw), max(int(math.Ceil(float64(im.h)*s)), bh)
	im.scaled(sw, sh).blit(p, x0, y0, sw, (sw-bw)/2, (sh-bh)/2, bw, bh, alpha)
	return float64(x0), float64(y0), float64(bw), float64(bh), true
}

// blit blends the w×h window of r (stride wide) starting at (cx, cy) onto the
// canvas with its top-left at (px, py), at opacity alpha times each pixel's own
// alpha.
func (r raster) blit(p *Pixels, px, py, stride, cx, cy, w, h int, alpha float64) {
	for y := 0; y < h; y++ {
		row := (cy+y)*stride + cx
		for x := 0; x < w; x++ {
			a := alpha
			if r.alpha != nil {
				a *= float64(r.alpha[row+x])
			}
			p.Blend(px+x, py+y, r.pix[row+x], a)
		}
	}
}

// scaled returns the image resampled to w×h.
func (im *decodedImage) scaled(w, h int) raster {
	im.mu.Lock()
	defer im.mu.Unlock()
	if out, ok := im.cache[[2]int{w, h}]; ok {
		return out
	}
	out := raster{pix: make([]RGB, w*h)}
	if im.alpha == nil {
		boxScale(out.pix, w, h, im.pix, im.w, im.h)
	} else {
		out.alpha = make([]float32, w*h)
		boxScaleAlpha(out, w, h, im.raster, im.w, im.h)
	}
	im.cache[[2]int{w, h}] = out
	return out
}

// boxScaleAlpha is boxScale for a raster with alpha. Colors are weighted by
// alpha, so a transparent pixel's color (often black) cannot darken the edge
// of a shape; the resampled alpha is the plain mean.
func boxScaleAlpha(dst raster, dw, dh int, src raster, sw, sh int) {
	for y := 0; y < dh; y++ {
		y0 := y * sh / dh
		y1 := max((y+1)*sh/dh, y0+1)
		for x := 0; x < dw; x++ {
			x0 := x * sw / dw
			x1 := max((x+1)*sw/dw, x0+1)
			var r, g, b, a float32
			for sy := y0; sy < y1; sy++ {
				for sx := x0; sx < x1; sx++ {
					i := sy*sw + sx
					c, ca := src.pix[i], src.alpha[i]
					r, g, b, a = r+c.R*ca, g+c.G*ca, b+c.B*ca, a+ca
				}
			}
			n := float32((y1 - y0) * (x1 - x0))
			if a > 0 {
				dst.pix[y*dw+x] = RGB{r / a, g / a, b / a}
			}
			dst.alpha[y*dw+x] = a / n
		}
	}
}

// boxScale resamples src (sw×sh) into dst (dw×dh): each dst pixel is the mean
// of the src pixels it covers, which is the nearest one when enlarging, so
// enlarged images stay crisp. It is the one shrinking filter for images,
// presenter previews and contact sheets.
func boxScale(dst []RGB, dw, dh int, src []RGB, sw, sh int) {
	for y := 0; y < dh; y++ {
		y0 := y * sh / dh
		y1 := max((y+1)*sh/dh, y0+1)
		for x := 0; x < dw; x++ {
			x0 := x * sw / dw
			x1 := max((x+1)*sw/dw, x0+1)
			var r, g, b float32
			for sy := y0; sy < y1; sy++ {
				for sx := x0; sx < x1; sx++ {
					c := src[sy*sw+sx]
					r, g, b = r+c.R, g+c.G, b+c.B
				}
			}
			n := float32((y1 - y0) * (x1 - x0))
			dst[y*dw+x] = RGB{r / n, g / n, b / n}
		}
	}
}
