package decker

import (
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"unicode/utf8"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

// A snapshot cell is cellW×cellH pixels.
const cellW, cellH = 8, 16

// writePNG saves frameImage's rendering of a frame to path, for
// `-snapshot -png file.png`.
func writePNG(frame string, w, h int, path string, t *Theme) error {
	return savePNG(frameImage(frame, w, h, t), path)
}

// frameImage paints a rendered frame the way a terminal would show it, to
// preview slides without one: block and box-drawing characters are drawn by
// hand, and others with a small bitmap font.
func frameImage(frame string, w, h int, t *Theme) *image.RGBA {
	cv := frameCanvas(frame, w, h)
	img := image.NewRGBA(image.Rect(0, 0, w*cellW, h*cellH))
	bgDefault := t.Background.Color()
	fgDefault := t.Text.Color()
	fillRect(img, img.Bounds(), bgDefault)

	d := &font.Drawer{Dst: img, Face: basicfont.Face7x13}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			cell := cv.CellAt(x, y)
			if cell == nil || cell.Width == 0 {
				continue
			}
			fg, bg := cell.Style.Fg, cell.Style.Bg
			if fg == nil {
				fg = fgDefault
			}
			if bg == nil {
				bg = bgDefault
			}
			r := image.Rect(x*cellW, y*cellH, (x+1)*cellW, (y+1)*cellH)
			fillRect(img, r, bg)
			if cell.Content == "" || cell.Content == " " {
				continue
			}
			r0, _ := utf8.DecodeRuneInString(cell.Content)
			if !drawBlockRune(img, r0, r, fg, bg) {
				d.Src = &image.Uniform{fg}
				d.Dot = fixed.P(r.Min.X, r.Min.Y+12)
				d.DrawString(cell.Content)
			}
		}
	}
	return img
}

func fillRect(img *image.RGBA, r image.Rectangle, c color.Color) {
	draw.Draw(img, r, &image.Uniform{c}, image.Point{}, draw.Src)
}

// previewShades are the opacities snapshots give the shade characters.
var previewShades = map[rune]float64{'░': 0.25, '▒': 0.5, '▓': 0.75}

// drawBlockRune paints block elements and box-drawing characters into a
// snapshot cell the way a terminal draws them (the bitmap font has none of
// them). It reports whether it handled the rune.
func drawBlockRune(img *image.RGBA, r rune, cell image.Rectangle, fg, bg color.Color) bool {
	w, h := cell.Dx(), cell.Dy()
	fill := func(x0, y0, x1, y1 int, c color.Color) {
		fillRect(img, image.Rect(cell.Min.X+x0, cell.Min.Y+y0, cell.Min.X+x1, cell.Min.Y+y1), c)
	}
	if m, ok := quadrants[r]; ok {
		for i, q := range [4][4]int{{0, 0, w / 2, h / 2}, {w / 2, 0, w, h / 2}, {0, h / 2, w / 2, h}, {w / 2, h / 2, w, h}} {
			if m&(0b1000>>i) != 0 {
				fill(q[0], q[1], q[2], q[3], fg)
			}
		}
		return true
	}
	if p, ok := previewShades[r]; ok {
		fill(0, 0, w, h, Mix(toRGB(bg), toRGB(fg), p).Color())
		return true
	}
	switch r {
	case '■':
		fill(0, 0, w, h, fg)
		return true
	case '▁':
		fill(0, h-h/8, w, h, fg)
		return true
	}
	arms, ok := boxArms[r]
	if !ok {
		return false
	}
	cx, cy := w/2, h/2
	for dir, weight := range arms {
		for i := 0; i < int(weight); i++ {
			o := 0
			if weight == 2 {
				o = 4*i - 2 // a double line is two lines, 4px apart
			}
			switch dir {
			case 0:
				fill(cx+o-1, 0, cx+o+1, cy+1, fg)
			case 1:
				fill(cx, cy+o-1, w, cy+o+1, fg)
			case 2:
				fill(cx+o-1, cy-1, cx+o+1, h, fg)
			default:
				fill(0, cy+o-1, cx+1, cy+o+1, fg)
			}
		}
	}
	return true
}

// writeSheet renders frames into one contact-sheet image, cols across,
// each shrunk by an integer factor.
func writeSheet(frames []string, w, h, cols, shrink int, path string, t *Theme) error {
	const gap = 6
	fw, fh := w*cellW/shrink, h*cellH/shrink
	rows := (len(frames) + cols - 1) / cols
	sheet := image.NewRGBA(image.Rect(0, 0, cols*(fw+gap)+gap, rows*(fh+gap)+gap))
	fillRect(sheet, sheet.Bounds(), color.Black)
	for i, fr := range frames {
		src := frameImage(fr, w, h, t)
		ox, oy := gap+(i%cols)*(fw+gap), gap+(i/cols)*(fh+gap)
		for y := 0; y < fh; y++ {
			for x := 0; x < fw; x++ {
				var r, g, b uint32
				for yy := 0; yy < shrink; yy++ {
					for xx := 0; xx < shrink; xx++ {
						c := src.RGBAAt(x*shrink+xx, y*shrink+yy)
						r, g, b = r+uint32(c.R), g+uint32(c.G), b+uint32(c.B)
					}
				}
				n := uint32(shrink * shrink)
				sheet.Set(ox+x, oy+y, RGB{float32(r / n), float32(g / n), float32(b / n)}.Color())
			}
		}
	}
	return savePNG(sheet, path)
}

func savePNG(img image.Image, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}
