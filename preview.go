package decker

import (
	"image"
	"sync"

	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
)

// previewKey names one preview: a slide at a step in a pw×ph box, drawn for a dw×dh deck.
type previewKey struct{ slide, step, pw, ph, dw, dh int }

// previewMu serializes slide drawing: slides are written for one caller at a
// time, and image previews draw in the background.
var previewMu sync.Mutex

// previewBox is the inside size of each preview frame: the deck's shape,
// leaving a few lines for notes. It is 0, 0 when previews don't fit.
func (p presenter) previewBox() (pw, ph int) {
	inner, rest := max(p.w-2*presMargin, 10), p.h-2-footerLines // 2: header and blank line
	dw, dh := p.deckSize()
	pw = (inner - presGutter - 4) / 2 // each frame adds 2 columns
	// A cell shows one pixel across and two down, like the deck's, so the
	// deck's shape in cells carries over directly.
	ph = pw * dh / dw
	if most := rest - 3 - 6; ph > most { // label + frame, and 6 lines of notes
		ph = most
		pw = ph * dw / dh
	}
	if ph < 5 || pw < 20 {
		return 0, 0
	}
	return pw, ph
}

func settledPixels(s Slide, step, w, h int, t *Theme) *Pixels {
	previewMu.Lock()
	defer previewMu.Unlock()
	return framePixels(renderSlide(s, Ctx{W: w, H: h, T: Settled, Step: step, StepT: Settled, Theme: t}), w, h, t)
}

// renderPreview draws s at a size it's designed for (240 cells wide, in the
// deck's shape), then shrinks it into pw×ph cells; drawing at preview size
// would lay the slide out for a tiny screen instead.
func renderPreview(s Slide, step, dw, dh, pw, ph int, t *Theme) string {
	const rw = 240
	rh := max(rw*dh/dw, 20)
	src := settledPixels(s, step, rw, rh, t)
	sc := NewScene(pw, ph, t)
	shrinkInto(src, sc.Px)
	return sc.Render()
}

// slideImage draws s settled at the deck's size, one image pixel per canvas pixel.
func slideImage(s Slide, step, dw, dh int, t *Theme) *image.RGBA {
	px := settledPixels(s, step, dw, dh, t)
	img := image.NewRGBA(image.Rect(0, 0, px.W, px.H))
	for i, c := range px.Pix {
		q := c.q()
		img.Pix[4*i], img.Pix[4*i+1], img.Pix[4*i+2], img.Pix[4*i+3] = q[0], q[1], q[2], 255
	}
	return img
}

func frameCanvas(frame string, w, h int) *lipgloss.Canvas {
	cv := lipgloss.NewCanvas(w, h)
	uv.NewStyledString(frame).Draw(cv, cv.Bounds())
	return cv
}

// framePixels turns a rendered frame back into pixels: a "▀" cell is its
// foreground over its background; other characters blend the two.
func framePixels(frame string, w, h int, t *Theme) *Pixels {
	cv := frameCanvas(frame, w, h)
	bgDefault, fgDefault := t.Background, t.Text
	px := NewPixels(w, 2*h, bgDefault)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			cell := cv.CellAt(x, y)
			if cell == nil || cell.Width == 0 {
				continue
			}
			fg, bg := fgDefault, bgDefault
			if cell.Style.Fg != nil {
				fg = toRGB(cell.Style.Fg)
			}
			if cell.Style.Bg != nil {
				bg = toRGB(cell.Style.Bg)
			}
			top, bot := bg, bg
			switch cell.Content {
			case "", " ":
			case "▀":
				top = fg
			case "▄":
				bot = fg
			case "█":
				top, bot = fg, fg
			default:
				top = Mix(bg, fg, 0.5)
				bot = top
			}
			px.Set(x, 2*y, top)
			px.Set(x, 2*y+1, bot)
		}
	}
	return px
}

func shrinkInto(src, dst *Pixels) {
	for y := 0; y < dst.H; y++ {
		y0, y1 := y*src.H/dst.H, max((y+1)*src.H/dst.H, y*src.H/dst.H+1)
		for x := 0; x < dst.W; x++ {
			x0, x1 := x*src.W/dst.W, max((x+1)*src.W/dst.W, x*src.W/dst.W+1)
			var r, g, b float32
			for yy := y0; yy < y1; yy++ {
				for xx := x0; xx < x1; xx++ {
					c := src.At(xx, yy)
					r, g, b = r+c.R, g+c.G, b+c.B
				}
			}
			n := float32((y1 - y0) * (x1 - x0))
			dst.Set(x, y, RGB{r / n, g / n, b / n})
		}
	}
}
