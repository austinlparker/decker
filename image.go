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

type decodedImage struct {
	w, h int
	pix  []RGB // nil when the file is missing or unreadable

	mu    sync.Mutex
	cache map[[2]int][]RGB
}

func (ims *Images) load(name string) *decodedImage {
	ims.mu.Lock()
	defer ims.mu.Unlock()
	if im, ok := ims.cache[name]; ok {
		return im
	}
	im := &decodedImage{cache: map[[2]int][]RGB{}}
	if data, err := fs.ReadFile(ims.fsys, path.Join(ims.dir, name)); err == nil {
		if src, _, err := image.Decode(bytes.NewReader(data)); err == nil {
			b := src.Bounds()
			im.w, im.h = b.Dx(), b.Dy()
			im.pix = make([]RGB, im.w*im.h)
			for y := 0; y < im.h; y++ {
				for x := 0; x < im.w; x++ {
					im.pix[y*im.w+x] = toRGB(src.At(b.Min.X+x, b.Min.Y+y))
				}
			}
		}
	}
	ims.cache[name] = im
	return im
}

// Has reports whether the image called name exists and decodes.
func (ims *Images) Has(name string) bool { return ims.load(name).pix != nil }

// Draw paints the image called name into the box (x, y, w, h), aspect kept,
// centered, at opacity alpha. Source pixels are averaged per canvas pixel and
// the resampled copy is cached per size. It returns the drawn rectangle, or
// ok=false (drawing nothing) if the image is missing.
func (ims *Images) Draw(p *Pixels, name string, x, y, w, h, alpha float64) (dx, dy, dw, dh float64, ok bool) {
	im := ims.load(name)
	if im.pix == nil || w < 1 || h < 1 {
		return 0, 0, 0, 0, false
	}
	s := min(w/float64(im.w), h/float64(im.h))
	sw, sh := max(int(float64(im.w)*s), 1), max(int(float64(im.h)*s), 1)
	scaled := im.scaled(sw, sh)
	ix, iy := int(math.Round(x+(w-float64(sw))/2)), int(math.Round(y+(h-float64(sh))/2))
	for py := 0; py < sh; py++ {
		for px := 0; px < sw; px++ {
			p.Blend(ix+px, iy+py, scaled[py*sw+px], alpha)
		}
	}
	return float64(ix), float64(iy), float64(sw), float64(sh), true
}

// scaled returns the image resampled to w×h.
func (im *decodedImage) scaled(w, h int) []RGB {
	im.mu.Lock()
	defer im.mu.Unlock()
	if out, ok := im.cache[[2]int{w, h}]; ok {
		return out
	}
	out := make([]RGB, w*h)
	boxScale(out, w, h, im.pix, im.w, im.h)
	im.cache[[2]int{w, h}] = out
	return out
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
