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

// sheetLayout places n tiles of w×h pixels on a contact sheet, sheetCols
// across, with gap pixels around and between them.
type sheetLayout struct{ n, w, h, gap int }

// size is the sheet's size in pixels.
func (l sheetLayout) size() (w, h int) {
	rows := (l.n + sheetCols - 1) / sheetCols
	return sheetCols*(l.w+l.gap) + l.gap, rows*(l.h+l.gap) + l.gap
}

// at is the top-left corner of tile i.
func (l sheetLayout) at(i int) (x, y int) {
	return l.gap + (i%sheetCols)*(l.w+l.gap), l.gap + (i/sheetCols)*(l.h+l.gap)
}

// writeSheet renders frames into one contact-sheet image, sheetCols across,
// each shrunk by an integer factor.
func writeSheet(frames []*grid, shrink int, path string) error {
	l := sheetLayout{n: len(frames), w: frames[0].W * cellW / shrink, h: frames[0].H * cellH / shrink, gap: 6}
	w, h := l.size()
	sheet := image.NewRGBA(image.Rect(0, 0, w, h))
	fillRect(sheet, sheet.Bounds(), color.Black)
	tile := make([]RGB, l.w*l.h)
	var full []RGB
	for i, fr := range frames {
		src := frameImage(fr)
		full = imageRGB(full, src)
		boxScale(tile, l.w, l.h, full, src.Rect.Dx(), src.Rect.Dy())
		x, y := l.at(i)
		pasteImage(sheet, x, y, tile, l.w)
	}
	return savePNG(sheet, path)
}

// pixelsImage converts a canvas to an image, for PNG.
func pixelsImage(p *Pixels) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, p.W, p.H))
	pasteImage(img, 0, 0, p.Pix, p.W)
	return img
}

// pasteImage writes the rows of src, w pixels wide, into img with their
// top-left at (x, y), which must leave them inside img.
func pasteImage(img *image.RGBA, x, y int, src []RGB, w int) {
	for r := 0; r*w < len(src); r++ {
		row := img.Pix[img.PixOffset(x, y+r):]
		for j, c := range src[r*w : (r+1)*w] {
			q := c.q()
			row[4*j], row[4*j+1], row[4*j+2], row[4*j+3] = q[0], q[1], q[2], 255
		}
	}
}

// paste copies the rows of src, w pixels wide, into p with their top-left
// at (x, y), which must leave them inside p.
func paste(p *Pixels, x, y int, src []RGB, w int) {
	for r := 0; r*w < len(src); r++ {
		copy(p.Pix[(y+r)*p.W+x:], src[r*w:(r+1)*w])
	}
}

// imageRGB reads img's colors into buf, grown if it is too small, and
// returns them.
func imageRGB(buf []RGB, img *image.RGBA) []RGB {
	n := len(img.Pix) / 4
	if cap(buf) < n {
		buf = make([]RGB, n)
	}
	buf = buf[:n]
	for j := range buf {
		buf[j] = rgbOf([3]uint8(img.Pix[4*j : 4*j+3]))
	}
	return buf
}

func savePNG(img image.Image, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}
