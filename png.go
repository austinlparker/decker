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

// writePNG saves frameImage's rendering to path (-snapshot -png).
func writePNG(g *grid, path string) error { return savePNG(frameImage(g), path) }

// frameImage paints a frame as a terminal would, to preview slides without
// one: block and box-drawing characters by hand, the rest in a bitmap font.
func frameImage(g *grid) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, g.W*cellW, g.H*cellH))
	d := &font.Drawer{Dst: img, Face: basicfont.Face7x13}
	for i := range g.Cells {
		c := &g.Cells[i]
		x, y := i%g.W, i/g.W
		fg, bg := rgba(c.fg), rgba(c.bg)
		r := image.Rect(x*cellW, y*cellH, (x+1)*cellW, (y+1)*cellH)
		fillRect(img, r, bg)
		if c.ch == "" || c.ch == " " {
			continue
		}
		r0, _ := utf8.DecodeRuneInString(c.ch)
		if !drawBlockRune(img, r0, r, fg, bg) {
			d.Src = &image.Uniform{fg}
			d.Dot = fixed.P(r.Min.X, r.Min.Y+12)
			d.DrawString(c.ch)
		}
	}
	return img
}

func rgba(q [3]uint8) color.RGBA { return color.RGBA{q[0], q[1], q[2], 255} }

func fillRect(img *image.RGBA, r image.Rectangle, c color.Color) {
	draw.Draw(img, r, &image.Uniform{c}, image.Point{}, draw.Src)
}

var previewShades = map[rune]float64{'░': 0.25, '▒': 0.5, '▓': 0.75}

// drawBlockRune paints block elements and box-drawing characters into a
// snapshot cell (the bitmap font has none); it reports whether it handled r.
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

// sheetCols is how many frames a contact sheet puts in a row.
const sheetCols = 4

// writeSheet renders frames into one contact-sheet image, sheetCols across,
// each shrunk by an integer factor.
func writeSheet(frames []*grid, shrink int, path string) error {
	const gap = 6
	fw, fh := frames[0].W*cellW/shrink, frames[0].H*cellH/shrink
	rows := (len(frames) + sheetCols - 1) / sheetCols
	sheet := image.NewRGBA(image.Rect(0, 0, sheetCols*(fw+gap)+gap, rows*(fh+gap)+gap))
	fillRect(sheet, sheet.Bounds(), color.Black)
	for i, fr := range frames {
		src := frameImage(fr)
		ox, oy := gap+(i%sheetCols)*(fw+gap), gap+(i/sheetCols)*(fh+gap)
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
